package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kopeisec/fp"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var LogWriter io.Writer

func init() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	logDir := filepath.Join(homeDir, ".anu", "log")

	if err := os.MkdirAll(logDir, 0755); err != nil {
		panic(err)
	}

	logFile := filepath.Join(logDir, fmt.Sprintf("anu.log"))

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		panic(err)
	}

	LogWriter = f
}

type Job struct {
	Type    string   `json:"type"`
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
	cmd := exec.Command(
		"ssh",
		"-p", fmt.Sprintf("%v", port),
		"-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", user, host), command)
	cmd.Stdin = nil
	cmd.Stdout = LogWriter
	cmd.Stderr = LogWriter
	return cmd.Run()
}

func runRsync(src string, dest string, port int) error {
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
	cmd.Stdout = LogWriter
	cmd.Stderr = LogWriter
	return cmd.Run()
}

func applyDockerCompose(job Job) error {
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

	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		fmt.Println("[ERROR] failed to create directory")
		return err
	}

	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s down", job.Workdir)); err != nil {
		fmt.Println("[WARN] run docker-compose down on remote failed.")
	}

	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		fmt.Println("[WARN] run rsync on remote failed.")
		return err
	}

	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s pull", job.Workdir)); err != nil {
		fmt.Println("[WARN] run docker-compose pull on remote failed.")
		return err
	}

	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s up -d", job.Workdir)); err != nil {
		fmt.Println("[WARN] run docker-compose up on remote failed.")
		return err
	}

	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("docker-compose --project-directory %s ps", job.Workdir)); err != nil {
		return err
	}

	return nil
}

func applyMake(job Job) error {
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

	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		return err
	}

	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		return err
	}

	return fp.Transform(
		fp.Wrap(job.Targets).Filter(fp.ConditionShouldNotEmpty),
		func(target string) error {
			if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("cd %s && make %s", job.Workdir, target)); err != nil {
				return err
			}
			return nil
		}).
		Filter(fp.ConditionHasError).
		Reduce(fp.AccumulateCombineErrors, nil)
}

func applyShell(job Job) error {
	if job.Host == "" {
		return errors.New("[ERROR] host is empty")
	}
	if job.Workdir == "" {
		return errors.New("[ERROR] workdir is empty")
	}

	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("mkdir -p %s/", job.Workdir)); err != nil {
		return err
	}

	fmt.Println("Sending files ...")
	if err := runRsync(".", fmt.Sprintf("%s@%s:%s/", job.User, job.Host, job.Workdir), job.Port); err != nil {
		return err
	}

	if err := runSSHCommand(job.Host, job.User, job.Port, fmt.Sprintf("cd %s && %s", job.Workdir, job.Shell)); err != nil {
		return err
	}

	fmt.Printf("[INFO] run shell [%s] on %s %s successfully.\n", job.Shell, job.Host, job.Workdir)
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
	configFile := filepath.Join(".lemuria", name+".yaml")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("[ERROR] config not found: %s", configFile)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}

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

	rootCmd.AddCommand(applyCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("[ERROR]", err.Error())
		os.Exit(1)
	}
}
