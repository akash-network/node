//go:build unix

package relayer

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDurableLockChild(t *testing.T) {
	path := os.Getenv("RELAYER_TEST_LOCK_CHILD")
	if path == "" {
		return
	}
	acquired, err := withFileLock(context.Background(), path, 0, func() error {
		fmt.Fprintln(os.Stdout, "locked")
		_, err := io.Copy(io.Discard, os.Stdin)
		return err
	})
	if !acquired || err != nil {
		t.Fatalf("child lock unavailable: %v", err)
	}
}

func lockChildCommand(t *testing.T, kind, path, mode string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if kind == "go" {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDurableLockChild$")
		cmd.Env = append(os.Environ(), "RELAYER_TEST_LOCK_CHILD="+path)
		return cmd
	}
	if kind == "python" {
		python, err := exec.LookPath("python3")
		if err != nil {
			t.Fatal("Python is required for legacy flock interoperability")
		}
		script := "import fcntl,os,sys\nfd=os.open(sys.argv[1],os.O_CREAT|os.O_RDWR,0o600)\ntry: fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)\nexcept BlockingIOError: sys.exit(73)\nprint('locked',flush=True)\nif sys.argv[2]=='hold': sys.stdin.buffer.read()\nos.close(fd)\n"
		return exec.CommandContext(ctx, python, "-c", script, path, mode)
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required to test the actual baseline TS lock helper")
	}
	module := filepath.Join(testRepositoryRoot(t), "contracts/solana/local/durable.ts")
	script := `const {withUploadLock}=await import(process.argv[1]); await withUploadLock(process.argv[2], async()=>{process.stdout.write("locked\n"); if(process.argv[3]==="hold") await new Promise(resolve=>{process.stdin.resume();process.stdin.on("end",resolve)});});`
	return exec.CommandContext(ctx, node, "--experimental-strip-types", "--input-type=module", "-e", script, module, path, mode)
}

func TestDurableFlockLegacyInteroperabilityAndSIGKILL(t *testing.T) {
	for _, kind := range []string{"go", "ts", "python"} {
		t.Run(kind+" holder", func(t *testing.T) {
			path := filepath.Join(durableTestDirectory(t), "payer.lock")
			cmd := lockChildCommand(t, kind, path, "hold")
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			t.Cleanup(func() {
				stdin.Close()
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			})
			if scanner := bufio.NewScanner(stdout); !scanner.Scan() || scanner.Text() != "locked" {
				_ = stdin.Close()
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				waited = true
				t.Fatalf("holder failed: %s", stderr.String())
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			acquired, err := withFileLock(context.Background(), path, 0, func() error {
				t.Error("Go entered a held legacy lock")
				return nil
			})
			if acquired || err != nil {
				t.Fatalf("contender %v %v", acquired, err)
			}
			if kind == "go" {
				for _, contender := range []string{"ts", "python"} {
					output, err := lockChildCommand(t, contender, path, "try").CombinedOutput()
					if err == nil || strings.Contains(string(output), "locked\n") {
						t.Fatalf("%s entered Go lock: %s %v", contender, output, err)
					}
				}
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = cmd.Wait()
			waited = true
			deadline := time.Now().Add(3 * time.Second)
			for {
				acquired, err = withFileLock(context.Background(), path, 0, func() error { return nil })
				if err != nil {
					t.Fatal(err)
				}
				if acquired {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("SIGKILL did not release test-owned lock")
				}
				time.Sleep(20 * time.Millisecond)
			}
			after, err := os.Stat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("lock inode changed or was unlinked")
			}
		})
	}
}

func TestDurableFlockNonreentrantWaitAndCancellation(t *testing.T) {
	path := filepath.Join(durableTestDirectory(t), "relay-budget.lock")
	acquired, err := withFileLock(context.Background(), path, 0, func() error {
		start := time.Now()
		inner, err := withFileLock(context.Background(), path, 0, func() error { return nil })
		if inner || err != nil || time.Since(start) > time.Second {
			t.Fatal("payer lock became reentrant or blocking")
		}
		start = time.Now()
		inner, err = withFileLock(context.Background(), path, 150*time.Millisecond, func() error { return nil })
		if inner || err != nil || time.Since(start) < 150*time.Millisecond || time.Since(start) > time.Second {
			t.Fatalf("budget wait %v %v %v", inner, err, time.Since(start))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		inner, err = withFileLock(ctx, path, 10*time.Second, func() error { return nil })
		if inner || err != context.DeadlineExceeded {
			t.Fatalf("lock cancellation %v %v", inner, err)
		}
		return nil
	})
	if !acquired || err != nil {
		t.Fatal(err)
	}
}
