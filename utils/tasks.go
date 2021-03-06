package utils

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
)

type (
	TaskMetadata struct {
		Context        string
		Skip           bool
		StdOutLogLevel logrus.Level
		StdErrLogLevel logrus.Level
	}

	TaskFunc func(*Task) error
	Command  *exec.Cmd

	Task struct {
		Command  Command
		Commands []Command
		Task     TaskFunc
		Tasks    []TaskFunc
		Metadata TaskMetadata
		Log      *logrus.Entry
	}

	RunAllTasksOptions struct {
	}
)

var TaskList []Task = []Task{}

func AddTask(command Task) []Task {
	TaskList = append(TaskList, command)

	return TaskList
}

func AddTasks(commands []Task) []Task {
	TaskList = append(TaskList, commands...)

	return TaskList
}

var DefaultRunAllTasksOptions = RunAllTasksOptions{}

func RunAllTasks(options RunAllTasksOptions) {
	if len(TaskList) == 0 {
		Log.WithField("context", "COMMAND").Fatalln("Task list is empty!")
	}

	for len(TaskList) != 0 {
		for _, task := range TaskList {
			if task.Metadata.StdOutLogLevel == 0 {
				task.Metadata.StdOutLogLevel = logrus.InfoLevel
			}

			if task.Metadata.StdErrLogLevel == 0 {
				task.Metadata.StdErrLogLevel = logrus.WarnLevel
			}

			task.Log = Log.WithField("context", task.Metadata.Context)

			if task.Metadata.Skip != true {
				if task.Tasks == nil {
					task.Tasks = []TaskFunc{}
				}

				if task.Task != nil {
					task.Tasks = append(task.Tasks, task.Task)
				}

				runTasks(&task, task.Tasks)

				if task.Commands == nil {
					task.Commands = []Command{}
				}

				if task.Command != nil {
					task.Commands = append(task.Commands, task.Command)
				}

				runCommands(&task, task.Commands)
			} else {
				Log.Warnln(fmt.Sprintf("Task skipped: %s", task.Metadata.Context))
			}

			TaskList = TaskList[1:]
		}
	}
}

func runTasks(task *Task, taskFuncs []TaskFunc) {
	for _, taskFunc := range taskFuncs {
		err := taskFunc(task)

		if err != nil {
			Log.WithField("context", "FAILED").
				Fatalln(fmt.Sprintf("$ Task > %s", err))
		}
	}
}

func runCommands(task *Task, commands []Command) {
	for _, command := range commands {
		cmd := strings.Join(command.Args, " ")

		Log.WithField("context", "RUN").
			Infoln(fmt.Sprintf("$ %s", cmd))

		command.Args = DeleteEmptyStringsFromSlice(command.Args)

		err := ExecuteAndPipeToLogger(command, task.Metadata)

		if err != nil {
			Log.WithField("context", "FAILED").
				Fatalln(fmt.Sprintf("$ %s > %s", cmd, err.Error()))
		} else {
			Log.WithField("context", "FINISH").Infoln(cmd)
		}
	}
}

func ExecuteAndPipeToLogger(cmd *exec.Cmd, context TaskMetadata) error {
	stdout, stdoutReader, stderr, stderrReader, err := CreateCommandReaders(cmd)

	if err != nil {
		Log.Fatalln(err)
	}

	defer stdout.Close()
	defer stderr.Close()

	if err := cmd.Start(); err != nil {
		Log.WithField("context", "FAILED").
			Fatalln(fmt.Sprintf("Can not start the command: %s", cmd))
	}

	go HandleOutputStreamReader(stdoutReader, context, context.StdOutLogLevel)
	go HandleOutputStreamReader(stderrReader, context, context.StdErrLogLevel)

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				Log.WithField("context", "EXIT").
					Debugln(fmt.Sprintf("Exit Status: %v", status.ExitStatus()))
			}
		}

		return err
	}

	return nil
}

func CreateCommandReaders(
	cmd *exec.Cmd,
) (io.ReadCloser, *bufio.Reader, io.ReadCloser, *bufio.Reader, error) {
	stdout, err := cmd.StdoutPipe()

	if err != nil {
		return nil, nil, nil, nil, errors.New(
			fmt.Sprintf("Failed creating command stdout pipe: %s", err),
		)
	}

	stdoutReader := bufio.NewReader(stdout)

	stderr, err := cmd.StderrPipe()

	if err != nil {
		return nil, nil, nil, nil, errors.New(
			fmt.Sprintf("Failed creating command stderr pipe: %s", err),
		)
	}

	stderrReader := bufio.NewReader(stderr)

	return stdout, stdoutReader, stderr, stderrReader, nil
}

func HandleOutputStreamReader(reader *bufio.Reader, context TaskMetadata, level logrus.Level) {
	var log *logrus.Entry = Log.WithFields(logrus.Fields{})

	if context.Context != "" {
		log = log.WithField("context", context.Context)
	}

	for {
		str, err := reader.ReadString('\n')

		if err != nil {
			break
		}

		log.Logln(level, str)
	}
}
