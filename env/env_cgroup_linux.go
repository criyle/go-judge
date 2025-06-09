package env

import (
	"context"
	"fmt"
	"os"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/criyle/go-judge/env/linuxcontainer"
	"github.com/criyle/go-sandbox/pkg/cgroup"
	ddbus "github.com/godbus/dbus/v5"
	"go.uber.org/zap"
)

func setupCgroup(c Config, logger *zap.Logger) (cgroup.Cgroup, *cgroup.Controllers, error) {
	prefix := c.CgroupPrefix
	t := cgroup.DetectedCgroupType

	ct, err := cgroup.GetAvailableController()
	if err != nil {
		logger.Error("failed to get available controllers", zap.Error(err))
		return nil, nil, err
	}

	if t == cgroup.TypeV2 {
		prefix, ct, err = setupCgroupV2(prefix, logger)
		if err != nil {
			return nil, nil, err
		}
	}

	return createAndNestCgroup(prefix, ct, c.NoFallback, logger)
}

func setupCgroupV2(prefix string, logger *zap.Logger) (string, *cgroup.Controllers, error) {
	logger.Info("running with cgroup v2, connecting systemd dbus to create cgroup")
	conn, err := getSystemdConnection()
	if err != nil {
		logger.Info("connecting to systemd dbus failed, assuming running in container, enable cgroup v2 nesting support and take control of the whole cgroupfs", zap.Error(err))
		return "", getControllersWithPrefix("", logger), nil
	}
	defer conn.Close()

	scopeName := prefix + ".scope"
	logger.Info("connected to systemd bus, attempting to create transient unit", zap.String("scopeName", scopeName))

	if err := startTransientUnit(conn, scopeName, logger); err != nil {
		return "", nil, err
	}

	scopeName, err = cgroup.GetCurrentCgroupPrefix()
	if err != nil {
		logger.Error("failed to get current cgroup prefix", zap.Error(err))
		return "", nil, err
	}
	logger.Info("current cgroup", zap.String("scope_name", scopeName))

	ct, err := cgroup.GetAvailableControllerWithPrefix(scopeName)
	if err != nil {
		logger.Error("failed to get available controller with prefix", zap.Error(err))
		return "", nil, err
	}
	return scopeName, ct, nil
}

func getSystemdConnection() (*dbus.Conn, error) {
	if os.Getuid() == 0 {
		return dbus.NewSystemConnectionContext(context.TODO())
	}
	return dbus.NewUserConnectionContext(context.TODO())
}

func startTransientUnit(conn *dbus.Conn, scopeName string, logger *zap.Logger) error {
	properties := []dbus.Property{
		dbus.PropDescription("go judge - a high performance sandbox service base on container technologies"),
		dbus.PropWants(scopeName),
		dbus.PropPids(uint32(os.Getpid())),
		newSystemdProperty("Delegate", true),
	}
	ch := make(chan string, 1)
	if _, err := conn.StartTransientUnitContext(context.TODO(), scopeName, "replace", properties, ch); err != nil {
		logger.Error("failed to start transient unit", zap.Error(err))
		return fmt.Errorf("failed to start transient unit: %w", err)
	}
	s := <-ch
	if s != "done" {
		logger.Error("starting transient unit returns error", zap.String("status", s))
		return fmt.Errorf("starting transient unit returns error: %s", s)
	}
	return nil
}

func getControllersWithPrefix(prefix string, logger *zap.Logger) *cgroup.Controllers {
	ct, err := cgroup.GetAvailableControllerWithPrefix(prefix)
	if err != nil {
		logger.Error("failed to get available controller with prefix", zap.Error(err))
		return nil
	}
	return ct
}

func createAndNestCgroup(prefix string, ct *cgroup.Controllers, noFallback bool, logger *zap.Logger) (cgroup.Cgroup, *cgroup.Controllers, error) {
	cgb, err := cgroup.New(prefix, ct)
	if err != nil {
		if os.Getuid() == 0 {
			logger.Error("failed to create cgroup", zap.String("prefix", prefix), zap.Error(err))
			return nil, nil, err
		}
		logger.Warn("not running in root and have no permission on cgroup, falling back to rlimit / rusage mode", zap.Error(err))
		if noFallback {
			return nil, nil, fmt.Errorf("failed to create cgroup with no fallback: %w", err)
		}
		return nil, nil, nil
	}
	logger.Info("creating nesting api cgroup", zap.Any("cgroup", cgb))
	if _, err = cgb.Nest("api"); err != nil {
		if os.Getuid() != 0 {
			logger.Warn("creating api cgroup with error, falling back to rlimit / rusage mode", zap.Error(err))
			cgb.Destroy()
			if noFallback {
				return nil, nil, fmt.Errorf("failed to create nesting api cgroup with no fallback: %w", err)
			}
			return nil, nil, nil
		}
	}

	logger.Info("creating containers cgroup")
	cg, err := cgb.New("containers")
	if err != nil {
		logger.Warn("creating containers cgroup with error, falling back to rlimit / rusage mode", zap.Error(err))
		if noFallback {
			return nil, nil, fmt.Errorf("failed to create containers cgroup with no fallback: %w", err)
		}
		return nil, nil, nil
	}
	if ct != nil && !ct.Memory {
		logger.Warn("memory cgroup is not enabled, falling back to rlimit / rusage mode")
		if noFallback {
			return nil, nil, fmt.Errorf("memory cgroup is not enabled with no fallback: %w", err)
		}
	}
	if ct != nil && !ct.Pids {
		logger.Warn("pid cgroup is not enabled, proc limit does not have effect")
	}
	return cg, ct, nil
}

func prepareCgroupPool(cgb cgroup.Cgroup, c Config) linuxcontainer.CgroupPool {
	if cgb != nil {
		return linuxcontainer.NewFakeCgroupPool(cgb, c.CPUCfsPeriod)
	}
	return nil
}

func getCgroupInfo(cgb cgroup.Cgroup, ct *cgroup.Controllers) (int, []string) {
	cgroupType := int(cgroup.DetectedCgroupType)
	if cgb == nil {
		cgroupType = 0
	}
	cgroupControllers := []string{}
	if ct != nil {
		cgroupControllers = ct.Names()
	}
	return cgroupType, cgroupControllers
}

func newSystemdProperty(name string, units any) dbus.Property {
	return dbus.Property{
		Name:  name,
		Value: ddbus.MakeVariant(units),
	}
}
