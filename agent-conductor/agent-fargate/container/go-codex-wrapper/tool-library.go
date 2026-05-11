package main

import (
	"fmt"
	"sort"
	"strings"

	"codex-wrapper/tools"
)

// ToolFunc is the in-process entrypoint shape for deterministic Codex tools.
// Tools are invoked by launching this wrapper binary with a --tool-name argument,
// so each function must be self-contained and communicate via stdout or events.
type ToolFunc func()

// Tool stores a registered tool implementation and its agent-facing guide text.
// The guide is printed by --print-tool-guides and tells Codex when and how to
// call the tool from inside the Fargate runtime.
type Tool struct {
	Run   ToolFunc
	Guide string
}

var toolRegistry = map[string]Tool{}

// registerBuiltinTools wires all built-in deterministic tools into the registry.
// This is called at process startup before either tool-mode or agent-mode logic.
// A missing registration makes the tool unavailable to Codex at runtime.
func registerBuiltinTools() {
	register_tool("ending", tools.Ending, tools.EndingGuide)
	register_tool("check-time", tools.CheckTime, tools.CheckTimeGuide)
	register_tool("print-tool-guides", printToolGuides, tools.PrintToolGuidesGuide)
}

// register_tool adds one named tool to the runtime registry.
// It panics on nil implementations or missing guides because that indicates a
// broken container image, not a recoverable agent task condition.
func register_tool(name string, run ToolFunc, guide string) {
	if run == nil {
		panic(fmt.Sprintf("tool %q has no run function", name))
	}

	guide = strings.TrimSpace(guide)
	if guide == "" {
		panic(fmt.Sprintf("tool %q has no guide", name))
	}

	toolRegistry[name] = Tool{
		Run:   run,
		Guide: guide,
	}
}

// runToolArgument executes a wrapper CLI tool when arg has the --tool-name shape.
// It returns true whenever the argument was treated as a tool request, including
// unknown tools, so main can avoid accidentally starting a nested Codex process.
func runToolArgument(arg string) bool {
	toolName, ok := strings.CutPrefix(arg, "--")
	if !ok || toolName == "" {
		return false
	}

	tool, ok := toolRegistry[toolName]
	if !ok {
		fmt.Printf("unknown tool: %s\navailable tools: %s\n", toolName, strings.Join(registeredToolNames(), ", "))
		return true
	}

	tool.Run()
	return true
}

// registeredToolNames returns deterministic registry ordering for help output.
// Stable output matters because Codex reads this text as task instructions.
func registeredToolNames() []string {
	names := make([]string, 0, len(toolRegistry))
	for name := range toolRegistry {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// printToolGuides prints every registered tool guide for Codex.
// The separator heading makes each guide visually distinct in terminal output,
// which helps the agent parse available actions during a headless run.
func printToolGuides() {
	for _, name := range registeredToolNames() {
		tool := toolRegistry[name]
		fmt.Printf("# =============================================================\n%s\n\n", tool.Guide)
	}
}
