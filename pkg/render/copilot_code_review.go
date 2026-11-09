package render

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotsettings"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	sharedrender "github.com/srz-zumix/go-gh-extension/pkg/render"
)

type outputWriter struct {
	io.Writer
}

func (outputWriter) Fd() uintptr {
	return ^uintptr(0)
}

type outputStream interface {
	io.Writer
	Fd() uintptr
}

type errorTrackingWriter struct {
	outputStream
	err error
}

func (w *errorTrackingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.outputStream.Write(p)
	if err == nil && n < len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}

// NewOutputRenderer routes renderer output to the command's streams.
func NewOutputRenderer(exporter cmdutil.Exporter, out, errOut io.Writer) *sharedrender.Renderer {
	r := sharedrender.NewRenderer(exporter)
	if file, ok := out.(*os.File); ok {
		r.IO.Out = file
		if file != os.Stdout {
			r.IO.SetStdoutTTY(false)
			r.IO.SetColorEnabled(false)
		}
	} else {
		r.IO.Out = outputWriter{out}
		r.IO.SetStdoutTTY(false)
		r.IO.SetColorEnabled(false)
	}
	if file, ok := errOut.(*os.File); ok {
		r.IO.ErrOut = file
	} else {
		r.IO.ErrOut = outputWriter{errOut}
		r.IO.SetStderrTTY(false)
	}
	return r
}

// RenderCopilotCodeReviewPolicy renders a single organization or enterprise policy.
func RenderCopilotCodeReviewPolicy(r *sharedrender.Renderer, policy *copilotsettings.Policy) error {
	if policy == nil || (policy.Organization == nil) == (policy.Enterprise == nil) {
		return fmt.Errorf("expected exactly one Copilot code review policy")
	}
	if policy.Organization != nil {
		p := policy.Organization
		if r.HasExporter() {
			return r.RenderExportedData(p)
		}
		return renderCodeReviewFields(r, [][]string{
			{"configured_state", nullableString(p.ConfiguredState)},
			{"effective_state", p.EffectiveState},
			{"editable", strconv.FormatBool(p.Editable)},
			{"managed_by", p.ManagedBy},
		})
	}
	if r.HasExporter() {
		return r.RenderExportedData(policy.Enterprise)
	}
	return renderCodeReviewFields(r, [][]string{
		{"policy_state", nullableString(policy.Enterprise.PolicyState)},
	})
}

// RenderCopilotCodeReviewRunner renders an organization's runner configuration.
func RenderCopilotCodeReviewRunner(r *sharedrender.Renderer, runner *gh.OrganizationCopilotCodeReviewRunner) error {
	if runner == nil {
		return fmt.Errorf("expected a non-nil Copilot code review runner")
	}
	if r.HasExporter() {
		return r.RenderExportedData(runner)
	}
	labels := "null"
	if runner.RunnerLabels != nil {
		labels = strings.Join(runner.RunnerLabels, ", ")
		if len(runner.RunnerLabels) == 0 {
			labels = "[]"
		}
	}
	configuredBy := runner.ConfiguredBy
	if configuredBy == "" {
		configuredBy = "(not reported)"
	}
	return renderCodeReviewFields(r, [][]string{
		{"runner_type", runner.RunnerType},
		{"runner_labels", labels},
		{"runner_group_name", nullableString(runner.RunnerGroupName)},
		{"allow_repository_customization", strconv.FormatBool(runner.AllowRepositoryCustomization)},
		{"configured_by", configuredBy},
	})
}

func renderCodeReviewFields(r *sharedrender.Renderer, rows [][]string) error {
	// The table renderer can discard errors returned by its output writer.
	original := r.IO.Out
	output := &errorTrackingWriter{outputStream: original}
	r.IO.Out = output
	defer func() { r.IO.Out = original }()

	table := r.NewTableWriter([]string{"FIELD", "VALUE"})
	for _, row := range rows {
		table.Append(row)
	}
	renderErr := table.Render()
	return errors.Join(renderErr, output.err)
}

func nullableString(value *string) string {
	if value == nil {
		return "null"
	}
	return *value
}
