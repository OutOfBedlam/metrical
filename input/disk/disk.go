package disk

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/OutOfBedlam/metric"
	"github.com/OutOfBedlam/metrical/registry"
	"github.com/shirou/gopsutil/v4/disk"
)

func init() {
	registry.Register("disk", (*Disk)(nil))
}

//go:embed "disk.toml"
var diskSampleConfig string

func (d *Disk) SampleConfig() string {
	return diskSampleConfig
}

var _ metric.Input = (*Disk)(nil)

type Disk struct {
	MountPoints     []string `toml:"mount_points"`
	IgnoreFS        []string `toml:"ignore_fs"`
	IgnoreMountOpts []string `toml:"ignore_mount_opts"`
}

func (d *Disk) Init() error {
	return nil
}

func (d *Disk) Gather(g *metric.Gather) error {
	disks, partitions, err := d.DiskUsage()
	if err != nil {
		return fmt.Errorf("error getting disk usage info: %w", err)
	}
	for i, du := range disks {
		if du.Total == 0 {
			// Skip dummy filesystem (procfs, cgroupfs, ...)
			continue
		}

		device := partitions[i].Device
		mountOpts := mountOptions(partitions[i].Opts)
		if slices.Contains(mountOpts, "nobrowse") {
			// Skip macOS volumes that are not user-visible
			continue
		}
		mountInfo := map[string]string{
			"path":   du.Path,
			"device": strings.ReplaceAll(device, "/dev/", ""),
			"fstype": du.Fstype,
			"mode":   mountOpts.mode(),
		}

		label, err := disk.Label(strings.TrimPrefix(device, "/dev/"))
		if err == nil && label != "" {
			mountInfo["label"] = label
		}

		var usedPercent float64
		if du.Used+du.Free > 0 {
			usedPercent = float64(du.Used) /
				(float64(du.Used) + float64(du.Free)) * 100
		}

		var inodesUsedPercent float64
		if du.InodesUsed+du.InodesFree > 0 {
			inodesUsedPercent = float64(du.InodesUsed) /
				(float64(du.InodesUsed) + float64(du.InodesFree)) * 100
		}

		name := "disk:" + du.Path + ":"
		g.Add(name+"total", float64(du.Total), metric.GaugeType(metric.UnitBytes))
		g.Add(name+"free", float64(du.Free), metric.GaugeType(metric.UnitBytes))
		g.Add(name+"used", float64(du.Used), metric.GaugeType(metric.UnitBytes))
		g.Add(name+"used_percent", usedPercent, metric.GaugeType(metric.UnitPercent))
		g.Add(name+"inodes_total", float64(du.InodesTotal), metric.GaugeType(metric.UnitShort))
		g.Add(name+"inodes_free", float64(du.InodesFree), metric.GaugeType(metric.UnitShort))
		g.Add(name+"inodes_used", float64(du.InodesUsed), metric.GaugeType(metric.UnitShort))
		g.Add(name+"inodes_used_percent", inodesUsedPercent, metric.GaugeType(metric.UnitPercent))
	}
	return nil
}

type mountOptions []string

func (opts mountOptions) mode() string {
	if opts.exists("rw") {
		return "rw"
	} else if opts.exists("ro") {
		return "ro"
	}
	return "unknown"
}

func (opts mountOptions) exists(opt string) bool {
	for _, o := range opts {
		if o == opt {
			return true
		}
	}
	return false
}

func (d *Disk) DiskUsage() ([]*disk.UsageStat, []*disk.PartitionStat, error) {
	mountPointFilter := d.MountPoints
	mountOptsExclude := d.IgnoreMountOpts
	fstypeExclude := d.IgnoreFS

	parts, err := disk.Partitions(true)
	if err != nil {
		return nil, nil, err
	}

	mountPointFilterSet := newSet()
	for _, filter := range mountPointFilter {
		mountPointFilterSet.add(filter)
	}
	mountOptFilterSet := newSet()
	for _, filter := range mountOptsExclude {
		mountOptFilterSet.add(filter)
	}
	fstypeExcludeSet := newSet()
	for _, filter := range fstypeExclude {
		fstypeExcludeSet.add(filter)
	}
	paths := newSet()
	for _, part := range parts {
		paths.add(part.Mountpoint)
	}

	// Autofs mounts indicate a potential mount, the partition will also be
	// listed with the actual filesystem when mounted.  Ignore the autofs
	// partition to avoid triggering a mount.
	fstypeExcludeSet.add("autofs")

	var usage []*disk.UsageStat
	var partitions []*disk.PartitionStat
	hostMountPrefix := os.Getenv("HOST_MOUNT_PREFIX")

partitionRange:
	for i := range parts {
		p := parts[i]

		for _, o := range p.Opts {
			if !mountOptFilterSet.empty() && mountOptFilterSet.has(o) {
				continue partitionRange
			}
		}
		// If there is a filter set and if the mount point is not a
		// member of the filter set, don't gather info on it.
		if !mountPointFilterSet.empty() && !mountPointFilterSet.has(p.Mountpoint) {
			continue
		}

		// If the mount point is a member of the exclude set,
		// don't gather info on it.
		if fstypeExcludeSet.has(p.Fstype) {
			continue
		}

		// If there's a host mount prefix use it as newer gopsutil version check for
		// the init's mount points usually pointing to the host-mountpoint but in the
		// container. This won't work for checking the disk-usage as the disks are
		// mounted at HOST_MOUNT_PREFIX...
		mountpoint := p.Mountpoint
		if hostMountPrefix != "" && !strings.HasPrefix(p.Mountpoint, hostMountPrefix) {
			mountpoint = filepath.Join(hostMountPrefix, p.Mountpoint)
			// Exclude conflicting paths
			if paths.has(mountpoint) {
				slog.Debug("[SystemPS] => conflicting mountpoint", "skipping", mountpoint, "hostMountPrefix", hostMountPrefix)
				continue
			}
		}

		du, err := disk.Usage(mountpoint)
		if err != nil {
			slog.Debug("[SystemPS] => unable to get disk usage", "mountpoint", mountpoint, "error", err)
			continue
		}

		du.Path = filepath.Join(string(os.PathSeparator), strings.TrimPrefix(p.Mountpoint, hostMountPrefix))
		du.Fstype = p.Fstype
		usage = append(usage, du)
		partitions = append(partitions, &p)
	}

	return usage, partitions, nil
}

type set struct {
	m map[string]struct{}
}

func (s *set) empty() bool {
	return len(s.m) == 0
}

func (s *set) add(key string) {
	s.m[key] = struct{}{}
}

func (s *set) has(key string) bool {
	var ok bool
	_, ok = s.m[key]
	return ok
}

func newSet() *set {
	s := &set{
		m: make(map[string]struct{}),
	}
	return s
}
