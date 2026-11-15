package relayer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Run treats context cancellation as a graceful stop request. A started tick
// finishes its durable work before shutdown; an idle watcher stops immediately.
func Run(ctx context.Context, args []string, build BuildInfo, streams Streams) error {
	deps := defaultDependencies(streams)
	if deps.streams.In == nil {
		deps.streams.In = strings.NewReader("")
	}
	working, err := os.Getwd()
	if err != nil {
		return err
	}
	// The legacy command imports common.ts before parsing even offline commands.
	if _, err := localNetworkConfig(os.LookupEnv, working); err != nil {
		return err
	}
	cmd, err := parseCommand(args)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	release := filepath.Dir(filepath.Dir(executable))
	handled, err := runOffline(cmd, build, deps.streams, release)
	if handled || err != nil {
		return err
	}
	var cfg config
	if cmd.name == "local" {
		workspace, err := sourceWorkspace(working)
		if err != nil {
			return err
		}
		if cmd.configPath == "" {
			cfg, err = localConfig(os.LookupEnv, workspace)
		} else {
			cfg, err = loadSourceConfig(cmd.configPath, working)
		}
		if err != nil {
			return err
		}
	} else {
		cfg, err = loadConfig(cmd.configPath, cmd.network, release)
		if err != nil {
			return err
		}
	}
	writeNotices(cfg, deps.streams.Err)
	work := context.WithoutCancel(ctx)
	s, err := newService(work, cfg, deps)
	if err != nil {
		return err
	}
	defer s.close()
	if cmd.name == "local" && cmd.configPath == "" {
		return s.runLocal(ctx, cmd)
	}
	if cmd.reset != "" {
		return s.resetBudget(work, cmd.reset)
	}
	return s.run(ctx, cmd.watch)
}

func sourceWorkspace(working string) (string, error) {
	for directory := working; ; directory = filepath.Dir(directory) {
		module, err := os.ReadFile(filepath.Join(directory, "go.mod"))
		if err == nil && strings.HasPrefix(string(module), "module pkg.akt.dev/node/v2\n") {
			return directory, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if filepath.Dir(directory) == directory {
			return "", fmt.Errorf("local commands must run inside the node source workspace")
		}
	}
}

func (s *service) run(ctx context.Context, watch bool) error {
	fields := map[string]any{
		"event":                  "relay-start",
		"payer":                  s.payer.PublicKey().String(),
		"programId":              s.cfg.program.String(),
		"akashChainId":           s.cfg.chainID,
		"maxLamportsPerTransfer": s.cfg.maxLamports,
		"stateDir":               s.cfg.stateDir,
	}
	if s.cfg.pinnedClient != nil {
		fields["lightClientPin"] = s.cfg.pinnedClient.String()
	}
	s.emit(false, fields)
	return runTicks(ctx, watch, s.cfg.poll, s.tick, s.deps)
}

func runTicks(ctx context.Context, watch bool, poll time.Duration, tick func(context.Context) error, deps dependencies) error {
	work := context.WithoutCancel(ctx)
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := tick(work); err != nil {
			if !watch {
				return err
			}
			fmt.Fprintln(deps.streams.Err, err)
		}
		if !watch || ctx.Err() != nil {
			return nil
		}
		if err := deps.sleep(ctx, poll); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}
