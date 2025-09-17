package opcua

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
)

func init() {
	registry.Register("opcua", (*OPCUA)(nil))
}

//go:embed "opcua.toml"
var opcuaSampleConfig string

func (o *OPCUA) SampleConfig() string {
	return opcuaSampleConfig
}

var _ metric.Input = (*OPCUA)(nil)

type OPCUA struct {
	Endpoint          string        `toml:"endpoint"`
	SecurityMode      string        `toml:"security_mode"`
	SecurityPolicy    string        `toml:"security_policy"`
	Certificate       string        `toml:"certificate"`
	PrivateKey        string        `toml:"private_key"`
	Nodes             []Node        `toml:"nodes"`
	ReadRetryInterval time.Duration `toml:"read_retry_interval"`
	ReadRetryCount    int           `toml:"read_retry_count"`
	ConnRetryInterval time.Duration `toml:"conn_retry_interval"`
	ConnRetryCount    int           `toml:"conn_retry_count"`

	ctx     context.Context
	client  *opcua.Client
	nodeIDs []*ua.NodeID
}

type Node struct {
	Name      string `toml:"name"`
	Namespace string `toml:"namespace"`
	IdType    string `toml:"id_type"`
	Id        string `toml:"id"`
}

func (o *OPCUA) Init() error {
	o.ctx = context.Background()
	if o.Endpoint == "" {
		return errors.New("endpoint is required")
	}
	if len(o.Nodes) == 0 {
		return errors.New("no nodes configured")
	}
	if o.ReadRetryInterval == 0 {
		o.ReadRetryInterval = 100 * time.Millisecond
	}
	if o.ConnRetryInterval == 0 {
		o.ConnRetryInterval = 1 * time.Second
	}
	if o.SecurityMode == "" {
		o.SecurityMode = "None"
	}

	for _, node := range o.Nodes {
		nodeID := fmt.Sprintf("ns=%s;%s=%s", node.Namespace, node.IdType, node.Id)
		id, err := ua.ParseNodeID(nodeID)
		if err != nil {
			return err
		}
		o.nodeIDs = append(o.nodeIDs, id)
	}
	if err := o.connect(); err != nil {
		return err
	}
	return nil
}

func (o *OPCUA) DeInit() error {
	o.disconnect()
	return nil
}

func (o *OPCUA) Gather(g *metric.Gather) error {
	var connRetry = 0
	for o.client == nil {
		if err := o.connect(); err != nil {
			slog.Debug("error connecting to OPC UA server", "error", err)
			if connRetry >= o.ConnRetryCount {
				return err
			}
			connRetry++
			continue
		}
	}
	for i, id := range o.nodeIDs {
		node := o.Nodes[i]
		req := &ua.ReadRequest{
			MaxAge:             0,
			TimestampsToReturn: ua.TimestampsToReturnBoth,
			NodesToRead: []*ua.ReadValueID{
				{
					NodeID:      id,
					AttributeID: ua.AttributeIDValue,
				},
			},
		}
		var rsp *ua.ReadResponse
		var retry = 0
		for {
			r, err := o.client.Read(o.ctx, req)
			if err == nil {
				rsp = r
				break
			}
			if retry >= o.ReadRetryCount {
				return fmt.Errorf("error reading node %s: %w", node.Name, err)
			}
			retry++
			switch {
			case err == io.EOF && o.client.State() != opcua.Closed:
				time.Sleep(o.ReadRetryInterval)
				continue
			case errors.Is(err, ua.StatusBadSessionIDInvalid),
				errors.Is(err, ua.StatusBadSessionNotActivated),
				errors.Is(err, ua.StatusBadSecureChannelIDInvalid):
				time.Sleep(o.ReadRetryInterval)
				continue
			default:
				return err
			}
		}
		value := rsp.Results[0].Value.Value()
		var val float64
		switch v := value.(type) {
		case float64:
			val = v
		case float32:
			val = float64(v)
		case int64:
			val = float64(v)
		case int32:
			val = float64(v)
		case int16:
			val = float64(v)
		case int8:
			val = float64(v)
		case uint64:
			val = float64(v)
		case uint32:
			val = float64(v)
		case uint16:
			val = float64(v)
		case uint8:
			val = float64(v)
		case bool:
			if v {
				val = 1
			} else {
				val = 0
			}
		default:
			// ignore other types for now
			continue
		}
		g.Add("opcua:"+node.Name, val, metric.GaugeType(metric.UnitShort))
	}
	return nil
}

func (o *OPCUA) connect() error {
	if o.client != nil {
		return nil
	}
	if c, err := opcua.NewClient(o.Endpoint, o.opts()...); err != nil {
		return err
	} else {
		o.client = c
	}
	if err := o.client.Connect(o.ctx); err != nil {
		o.client = nil
		return err
	}
	return nil
}

func (o *OPCUA) disconnect() {
	if o.client != nil {
		o.client.Close(o.ctx)
		o.client = nil
	}
}

func (o *OPCUA) opts() []opcua.Option {
	var ret []opcua.Option
	switch o.SecurityMode {
	case "None":
		ret = append(ret,
			opcua.SecurityMode(ua.MessageSecurityModeNone),
			opcua.AuthAnonymous())
	case "Sign":
		ret = append(ret,
			opcua.SecurityMode(ua.MessageSecurityModeSign),
			opcua.AuthAnonymous())
	case "SignAndEncrypt":
		ret = append(ret,
			opcua.SecurityMode(ua.MessageSecurityModeSignAndEncrypt),
			opcua.AuthAnonymous())
		switch strings.ToUpper(strings.TrimSpace(o.SecurityPolicy)) {
		case "NONE":
			ret = append(ret, opcua.SecurityPolicy(ua.SecurityPolicyURINone))
		case "PREFIX":
			ret = append(ret, opcua.SecurityPolicy(ua.SecurityPolicyURIPrefix))
		case "BASIC128RSA15":
			ret = append(ret, opcua.SecurityPolicy(ua.SecurityPolicyURIBasic128Rsa15))
		case "BASIC256":
			ret = append(ret, opcua.SecurityPolicy(ua.SecurityPolicyURIBasic256))
		case "BASIC256SHA256":
			ret = append(ret, opcua.SecurityPolicy(ua.SecurityPolicyURIBasic256Sha256))
		case "AES128SHA256RSAOAP":
			ret = append(ret, opcua.SecurityPolicy(ua.SecurityPolicyURIAes128Sha256RsaOaep))
		case "AES256SHA256RSAPSS":
			ret = append(ret, opcua.SecurityPolicy(ua.SecurityPolicyURIAes256Sha256RsaPss))
		}
		// Load client certificate and private key
		ret = append(ret, opcua.CertificateFile(o.Certificate))
		ret = append(ret, opcua.PrivateKeyFile(o.PrivateKey))

	default:
		ret = append(ret, opcua.SecurityMode(ua.MessageSecurityModeNone))
	}
	return ret
}
