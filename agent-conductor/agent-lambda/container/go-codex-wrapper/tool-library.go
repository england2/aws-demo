package main

import (
	"codex-wrapper/tools"
	"fmt"
	"sort"
	"strings"
)

type ToolFunc func()

var toolRegistry = map[string]ToolFunc{}

func registerBuiltinTools() {
	register_tool("ending", tools.Ending)
}

func register_tool(name string, tool ToolFunc) {
	toolRegistry[name] = tool
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

	tool()
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
