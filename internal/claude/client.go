package claude

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

type Executor interface {
	Execute(name string, args ...string) ([]byte, error)
}

type osExecutor struct{}

func (e *osExecutor) Execute(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

type Client struct {
	executor  Executor
	sessionID string
	model     string
	sleep     func(time.Duration)
}

func New(executor Executor, sessionID, model string) *Client {
	return &Client{
		executor:  executor,
		sessionID: sessionID,
		model:     model,
		sleep:     time.Sleep,
	}
}

func NewDefault(sessionID, model string) *Client {
	return New(&osExecutor{}, sessionID, model)
}

func (c *Client) Run(prompt, schema string) (*Response, error) {
	return c.runWith("--session-id", prompt, schema)
}

func (c *Client) Resume(prompt, schema string) (*Response, error) {
	return c.runWith("--resume", prompt, schema)
}

func (c *Client) RunWithRetry(prompt, schema string, prompter func(string) (string, error)) (*Response, error) {
	resp, err := c.Run(prompt, schema)
	if err != nil {
		return nil, err
	}
	return c.retryOnError(resp, prompt, schema, prompter)
}

func (c *Client) ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) (*Response, error) {
	resp, err := c.Resume(prompt, schema)
	if err != nil {
		return nil, err
	}
	return c.retryOnError(resp, prompt, schema, prompter)
}

func (c *Client) runWith(sessionFlag, prompt, schema string) (*Response, error) {
	args := []string{
		"-p",
		"--dangerously-skip-permissions",
		sessionFlag, c.sessionID,
		"--output-format", "json",
		"--model", c.model,
	}
	if schema != "" {
		args = append(args, "--json-schema", schema)
	}
	args = append(args, prompt)
	out, err := c.executor.Execute("claude", args...)
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("claude execute: %w", err)
	}
	return ParseResponse(out)
}

func (c *Client) retryOnError(resp *Response, prompt, schema string, prompter func(string) (string, error)) (*Response, error) {
	for resp.IsError {
		if rl, ok := ParseRateLimitError(resp.Result); ok {
			c.countdownSleep(rl.WaitDuration)
		} else {
			fmt.Fprintf(os.Stderr, "Claude error: %s\n", resp.Result)
			answer, err := prompter("continue?[yn] ")
			if err != nil {
				return nil, err
			}
			if answer == "n" {
				return nil, fmt.Errorf("user aborted after claude error")
			}
		}
		var err error
		resp, err = c.Resume(prompt, schema)
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (c *Client) countdownSleep(d time.Duration) {
	remaining := d
	for remaining > 0 {
		h := int(remaining.Hours())
		m := int(remaining.Minutes()) % 60
		s := int(remaining.Seconds()) % 60
		fmt.Printf("\rWaiting %d hours, %d minutes, and %d seconds ...", h, m, s)
		c.sleep(time.Second)
		remaining -= time.Second
	}
	fmt.Println("\n*Yawns*... Okay, time to get up.")
}
