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

func logf(level, format string, args ...interface{}) string {
	return fmt.Sprintf("%s [%s] %s", time.Now().Format("2006-01-02 15:04:05 -0700"), level, fmt.Sprintf(format, args...))
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
	fmt.Fprintln(LogWriterStdout, logf("INFO", "Running SSH command on %s@%s:%d", user, host, port))
	fmt.Fprintln(LogWriterStdout, logf("INFO", "Command: %s", command))
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
	fmt.Fprintln(LogWriterStdout, logf("INFO", "Syncing files: %s -> %s", src, dest))
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
		fmt.Fprintln(LogWriterStdout, logf("INFO", "File sync completed"))
	}
	return err
}

func applyDockerCompose(job Job) error {
	fmt.Println(logf("INFO", "Starting docker-compose deployment on %s", job.Host))
	if job.Host == "" {
		return errors.New(logf("ERROR", "host is empty"))
	}
	if job.Workdir == "" {
		return errors.New(logf("ERROR", "workdir is empty"))
	}

	if job.User == "" {
		return errors.New(logf("ERROR", "user is empty"))
	}

	if job.Port == 0 {
		return errors.New(logf("ERROR", "port is empty"))
	}

	fmt.Println(logf("INFO", "Creating remote directory: %s", job.Workdir))
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		fmt.Println(logf("ERROR", "failed to create directory"))
		return err
	}

	if !fastMode {
		fmt.Println(logf("INFO", "Stopping existing containers..."))
		if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s down", job.Workdir)); err != nil {
			fmt.Println(logf("WARN", "run docker-compose down on remote failed."))
		}
	}

	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		fmt.Println(logf("ERROR", "run rsync on remote failed."))
		return err
	}

	if job.Pull == "always" {
		fmt.Println(logf("INFO", "Pulling latest images..."))
		if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s pull", job.Workdir)); err != nil {
			fmt.Println(logf("ERROR", "run docker-compose pull on remote failed."))
			return err
		}
	}

	fmt.Println(logf("INFO", "Starting containers..."))
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s up -d", job.Workdir)); err != nil {
		fmt.Println(logf("ERROR", "run docker-compose up on remote failed."))
		return err
	}

	fmt.Println(logf("INFO", "Checking container status..."))
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s ps", job.Workdir)); err != nil {
		return err
	}

	return nil
}

func applyKubernetesYAML(job Job) error {
	fmt.Println(logf("INFO", "Starting kubernetes yaml deployment on %s", job.Host))
	if job.Host == "" {
		return errors.New(logf("ERROR", "host is empty"))
	}
	if job.Workdir == "" {
		return errors.New(logf("ERROR", "workdir is empty"))
	}

	if job.User == "" {
		return errors.New(logf("ERROR", "user is empty"))
	}

	if job.Port == 0 {
		return errors.New(logf("ERROR", "port is empty"))
	}

	fmt.Println(logf("INFO", "Creating remote directory: %s", job.Workdir))
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		fmt.Println(logf("ERROR", "failed to create directory"))
		return err
	}

	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		fmt.Println(logf("ERROR", "run rsync on remote failed."))
		return err
	}

	applyCmd := fmt.Sprintf(
		"cd %s && files=$(find . -type f \\( -name '*.yaml' -o -name '*.yml' \\) | sort) && if [ -z \"$files\" ]; then echo 'no kubernetes yaml files found'; exit 1; fi && echo \"$files\" | while IFS= read -r file; do kubectl apply -f \"$file\"; done",
		job.Workdir,
	)
	fmt.Println(logf("INFO", "Applying kubernetes yaml files..."))
	if err := runSSHCommand(job.Host, job.User, job.Port, applyCmd); err != nil {
		fmt.Println(logf("ERROR", "run kubectl apply on remote failed."))
		return err
	}

	return nil
}

func applyMake(job Job) error {
	fmt.Println(logf("INFO", "Starting make deployment on %s", job.Host))
	if job.Host == "" {
		return errors.New(logf("ERROR", "host is empty"))
	}

	if job.Workdir == "" {
		return errors.New(logf("ERROR", "workdir is empty"))
	}

	if job.User == "" {
		return errors.New(logf("ERROR", "user is empty"))
	}

	if job.Port == 0 {
		return errors.New(logf("ERROR", "port is empty"))
	}

	fmt.Println(logf("INFO", "Creating remote directory: %s", job.Workdir))
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		return err
	}

	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		return err
	}

	return fp.Transform(
		fp.Wrap(job.Targets).Filter(fp.ConditionShouldNotEmpty),
		func(target string) error {
			fmt.Println(logf("INFO", "Running make target: %s", target))
			if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("cd %s && make %s", job.Workdir, target)); err != nil {
				return err
			}
			fmt.Println(logf("INFO", "Make target '%s' completed", target))
			return nil
		}).
		Filter(fp.ConditionHasError).
		Reduce(fp.AccumulateCombineErrors, nil)
}

func applyShell(job Job) error {
	fmt.Println(logf("INFO", "Starting shell deployment on %s", job.Host))
	if job.Host == "" {
		return errors.New(logf("ERROR", "host is empty"))
	}
	if job.Workdir == "" {
		return errors.New(logf("ERROR", "workdir is empty"))
	}

	fmt.Println(logf("INFO", "Creating remote directory: %s", job.Workdir))
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		return err
	}

	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		return err
	}

	fmt.Println(logf("INFO", "Executing shell command: %s", job.Shell))
	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("cd %s && %s", job.Workdir, job.Shell)); err != nil {
		return err
	}

	fmt.Println(logf("INFO", "Shell command completed successfully on %s", job.Host))
	return nil
}

func applyJson(job Job) error {

	if len(job.Hosts) > 0 {
		return fp.TransformAsync(job.Hosts, func(host string) error {
			return applyJson(Job{
				Type:    job.Type,
				Pull:    job.Pull,
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
			fmt.Println(logf("FAIL", "%v", job.Host))
			return err
		}
		fmt.Println(logf(" OK ", "%v", job.Host))
		return nil
	case "make":
		if err := applyMake(job); err != nil {
			fmt.Println(logf("FAIL", "%v", job.Host))
			return err
		}
		fmt.Println(logf(" OK ", "%v", job.Host))
		return nil
	case "shell":
		if err := applyShell(job); err != nil {
			fmt.Println(logf("FAIL", "%v", job.Host))
			return err
		}
		fmt.Println(logf(" OK ", "%v", job.Host))
		return nil
	case "k8s":
		if err := applyKubernetesYAML(job); err != nil {
			fmt.Println(logf("FAIL", "%v", job.Host))
			return err
		}
		fmt.Println(logf(" OK ", "%v", job.Host))
		return nil
	default:
		return errors.New(logf("ERROR", "unsupported type: %s", job.Type))
	}
}

func apply(name string) error {
	fmt.Println(logf("INFO", "Loading configuration: %s", name))
	configFile := filepath.Join(".lemuria", name+".yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf(logf("ERROR", "config not found: %s", configFile))
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	fmt.Println(logf("INFO", "Configuration loaded, found %d job(s)", len(cfg.Jobs)))

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
			return fmt.Errorf(logf("ERROR", "failed to apply on %v, err: %%w", job.Host), err)
		}
		return nil
	}).Filter(fp.ConditionHasError).Reduce(fp.AccumulateCombineErrors, nil)
}

var fastMode bool

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
	applyCmd.PersistentFlags().BoolVar(&fastMode, "fast", false, "Fast mode")

	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("anu %v\n", Version)
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
		fmt.Println(logf("ERROR", "%s", err.Error()))
		os.Exit(1)
	}
}
