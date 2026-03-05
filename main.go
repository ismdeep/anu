package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/ismdeep/anu/version"
	"github.com/kopeisec/fp"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	LogWriterStdout io.Writer
	LogWriterStderr io.Writer
)

func init() {
	LogWriterStdout = os.Stdout
	LogWriterStderr = os.Stderr
}

type indentWriter struct {
	writer io.Writer
	prefix string
}

func (w *indentWriter) Write(p []byte) (n int, err error) {
	scanner := bufio.NewScanner(bufio.NewReader(bufio.NewReader(nil)))
	for i := 0; i < len(p); {
		lineEnd := i
		for lineEnd < len(p) && p[lineEnd] != '\n' {
			lineEnd++
		}
		if lineEnd < len(p) {
			lineEnd++
		}
		if lineEnd > i {
			w.writer.Write([]byte(w.prefix))
			w.writer.Write(p[i:lineEnd])
		}
		i = lineEnd
	}
	_ = scanner
	return len(p), nil
}

type Job struct {
	Type    string   `json:"type"`
	Pull    string   `json:"pull"`
	Host    string   `json:"host"`
	Hosts   []string `json:"hosts"`
	User    string   `json:"user"`
	Port    int      `json:"port"`
	Workdir string   `json:"workdir"`
	Shell   string   `json:"shell"`
	Targets []string `json:"targets"`
}

type Config struct {
	Jobs []Job `json:"jobs"`
}

func runSSHCommand(host string, user string, port int, command string) error {
	fmt.Fprintf(LogWriterStdout, "[INFO] Running SSH command on %s@%s:%d\n", user, host, port)
	fmt.Fprintf(LogWriterStdout, "[INFO] Command: %s\n", command)
	cmd := exec.Command(
		"ssh",
		"-p", fmt.Sprintf("%v", port),
		"-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", user, host), command)
	cmd.Stdin = nil
	cmd.Stdout = &indentWriter{writer: LogWriterStdout, prefix: "  "}
	cmd.Stderr = &indentWriter{writer: LogWriterStderr, prefix: "  "}
	return cmd.Run()
}

func runRsync(src string, dest string, port int) error {
	fmt.Fprintf(LogWriterStdout, "[INFO] Syncing files: %s -> %s\n", src, dest)
	cmd := exec.Command(
		"rsync",
		"-a",
		"-r",
		"--no-i-r",
		"--info=progress2",
		"--info=name0",
		"--no-owner",
		"--no-group",
		"--no-perms",
		"--delete",
		"-e", "ssh -o StrictHostKeyChecking=no",
		"--exclude-from=.lemuria/rsync-exclude-list",
		"--exclude=.lemuria",
		"-e", fmt.Sprintf("ssh -p %v", port), src, dest)
	cmd.Stdin = nil
	cmd.Stdout = &indentWriter{writer: LogWriterStdout, prefix: "  "}
	cmd.Stderr = &indentWriter{writer: LogWriterStderr, prefix: "  "}
	err := cmd.Run()
	if err == nil {
		fmt.Fprintf(LogWriterStdout, "[INFO] File sync completed\n")
	}
	return err
}

func applyDockerCompose(job Job) error {
	fmt.Printf("[INFO] Starting docker-compose deployment on %s\n", job.Host)
	if job.Host == "" {
		return errors.New("[ERROR] host is empty")
	}
	if job.Workdir == "" {
		return errors.New("[ERROR] workdir is empty")
	}

	if job.User == "" {
		return errors.New("[ERROR] user is empty")
	}

	if job.Port == 0 {
		return errors.New("[ERROR] port is empty")
	}

	fmt.Printf("[INFO] Creating remote directory: %s\n", job.Workdir)
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		fmt.Println("[ERROR] failed to create directory")
		return err
	}

	fmt.Println("[INFO] Stopping existing containers...")
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s down", job.Workdir)); err != nil {
		fmt.Println("[WARN] run docker-compose down on remote failed.")
	}

	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		fmt.Println("[ERROR] run rsync on remote failed.")
		return err
	}

	if job.Pull == "always" {
		fmt.Println("[INFO] Pulling latest images...")
		if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s pull", job.Workdir)); err != nil {
			fmt.Println("[ERROR] run docker-compose pull on remote failed.")
			return err
		}
	}

	fmt.Println("[INFO] Starting containers...")
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s up -d", job.Workdir)); err != nil {
		fmt.Println("[ERROR] run docker-compose up on remote failed.")
		return err
	}

	fmt.Println("[INFO] Checking container status...")
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s ps", job.Workdir)); err != nil {
		return err
	}

	return nil
}

func applyMake(job Job) error {
	fmt.Printf("[INFO] Starting make deployment on %s\n", job.Host)
	if job.Host == "" {
		return errors.New("[ERROR] host is empty")
	}

	if job.Workdir == "" {
		return errors.New("[ERROR] workdir is empty")
	}

	if job.User == "" {
		return errors.New("[ERROR] user is empty")
	}

	if job.Port == 0 {
		return errors.New("[ERROR] port is empty")
	}

	fmt.Printf("[INFO] Creating remote directory: %s\n", job.Workdir)
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		return err
	}

	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		return err
	}

	return fp.Transform(
		fp.Wrap(job.Targets).Filter(fp.ConditionShouldNotEmpty),
		func(target string) error {
			fmt.Printf("[INFO] Running make target: %s\n", target)
			if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("cd %s && make %s", job.Workdir, target)); err != nil {
				return err
			}
			fmt.Printf("[INFO] Make target '%s' completed\n", target)
			return nil
		}).
		Filter(fp.ConditionHasError).
		Reduce(fp.AccumulateCombineErrors, nil)
}

func applyShell(job Job) error {
	fmt.Printf("[INFO] Starting shell deployment on %s\n", job.Host)
	if job.Host == "" {
		return errors.New("[ERROR] host is empty")
	}
	if job.Workdir == "" {
		return errors.New("[ERROR] workdir is empty")
	}

	fmt.Printf("[INFO] Creating remote directory: %s\n", job.Workdir)
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		return err
	}

	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		return err
	}

	fmt.Printf("[INFO] Executing shell command: %s\n", job.Shell)
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("cd %s && %s", job.Workdir, job.Shell)); err != nil {
		return err
	}

	fmt.Printf("[INFO] Shell command completed successfully on %s\n", job.Host)
	return nil
}

func applyJson(job Job) error {

	if len(job.Hosts) > 0 {
		return fp.TransformAsync(job.Hosts, func(host string) error {
			return applyJson(Job{
				Type:    job.Type,
				Host:    host,
				Hosts:   nil,
				User:    job.User,
				Port:    job.Port,
				Workdir: job.Workdir,
				Shell:   job.Shell,
				Targets: job.Targets,
			})
		}).Reduce(fp.AccumulateCombineErrors, nil)
	}

	switch job.Type {
	case "docker-compose":
		if err := applyDockerCompose(job); err != nil {
			fmt.Printf("[FAIL] %v\n", job.Host)
			return err
		}
		fmt.Printf("[ OK ] %v\n", job.Host)
		return nil
	case "make":
		if err := applyMake(job); err != nil {
			fmt.Printf("[FAIL] %v\n", job.Host)
			return err
		}
		fmt.Printf("[ OK ] %v\n", job.Host)
		return nil
	case "shell":
		if err := applyShell(job); err != nil {
			fmt.Printf("[FAIL] %v\n", job.Host)
			return err
		}
		fmt.Printf("[ OK ] %v\n", job.Host)
		return nil
	default:
		return errors.New("[ERROR] unsupported type: " + job.Type)
	}
}

func apply(name string) error {
	fmt.Printf("[INFO] Loading configuration: %s\n", name)
	configFile := filepath.Join(".lemuria", name+".yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("[ERROR] config not found: %s", configFile)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	fmt.Printf("[INFO] Configuration loaded, found %d job(s)\n", len(cfg.Jobs))

	jobs := fp.Wrap(cfg.Jobs).Map(func(job Job) Job {
		// 端口默认为 22
		if job.Port == 0 {
			job.Port = 22
		}
		// 用户默认为 root
		if job.User == "" {
			job.User = "root"
		}
		return job
	})

	return fp.TransformAsync(jobs, func(job Job) error {
		if err := applyJson(job); err != nil {
			return fmt.Errorf("[ERROR] failed to apply on %v, err: %w", job.Host, err)
		}
		return nil
	}).Filter(fp.ConditionHasError).Reduce(fp.AccumulateCombineErrors, nil)
}

func main() {
	var name string

	var rootCmd = &cobra.Command{
		Use:   "anu",
		Short: "A command line tool for managing deployments",
		Run: func(cmd *cobra.Command, args []string) {
			if err := apply(name); err != nil {
				fmt.Println(err)
			}
		},
	}

	var applyCmd = &cobra.Command{
		Use:   "apply [name]",
		Short: "Apply the deployment",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			var nameList []string
			switch len(args) {
			case 0:
				nameList = append(nameList, "default")
			default:
				nameList = args
			}
			if err := fp.Transform(nameList, func(name string) error {
				return apply(name)
			}).Filter(fp.ConditionHasError).Reduce(fp.AccumulateCombineErrors, nil); err != nil {
				fmt.Println(err)
			}
		},
	}

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("anu %v\n", version.Version)
			buildInfo, ok := debug.ReadBuildInfo()
			if ok {
				fmt.Printf("go version: %v\n", buildInfo.GoVersion)
				for _, kv := range buildInfo.Settings {
					switch kv.Key {
					case "vcs.revision":
						fmt.Printf("commit id: %v\n", kv.Value)
					case "vcs.time":
						LastCommit, _ := time.Parse(time.RFC3339, kv.Value)
						fmt.Printf("commit time: %v\n", LastCommit)
					}
				}
			}
		},
	}

	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(versionCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("[ERROR]", err.Error())
		os.Exit(1)
	}
}
