package main

import (
	"codex-wrapper/tools"
	"fmt"
	"sort"
	"strings"
)

type ToolFunc func()

type Tool struct {
	Run   ToolFunc
	Guide string
}

var toolRegistry = map[string]Tool{}

func registerBuiltinTools() {
	register_tool("ending", tools.Ending, tools.EndingGuide)
	register_tool("check-time", tools.CheckTime, tools.CheckTimeGuide)
	register_tool("print-tool-guides", printToolGuides, tools.PrintToolGuidesGuide)
}

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

func registeredToolNames() []string {
	names := make([]string, 0, len(toolRegistry))
	for name := range toolRegistry {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

func printToolGuides() {
	for _, name := range registeredToolNames() {
		tool := toolRegistry[name]
		fmt.Printf("# =============================================================\n%s\n\n", tool.Guide)
	}
}
