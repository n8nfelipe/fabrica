package securitygateway

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
)

type stringList []string

func (values *stringList) String() string { return fmt.Sprint([]string(*values)) }
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

// RunCLI serves the private stdio gateway mode used by OpenCode MCP.
func RunCLI(args []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("security-gateway", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", "", "canonical project workspace")
	phase := flags.String("phase", "", "specification or implementation")
	taskID := flags.Int64("task-id", 0, "task identifier")
	auditFile := flags.String("audit-file", "", "private violation audit file")
	var writablePaths stringList
	flags.Var(&writablePaths, "write-path", "approved write path; may be repeated")
	if err := flags.Parse(args); err != nil {
		return errors.New("invalid security gateway arguments")
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected security gateway arguments")
	}
	policy, err := NewPolicy(*root, *phase, *taskID, writablePaths, *auditFile)
	if err != nil {
		return err
	}
	return New(policy).Serve(context.Background(), input, output)
}
