package browser

// variables.go — ydf_browser/variables.py 的 Go 移植。
// 支持 {{name}} 占位符解析、运行时捕获变量、--var/变量文件加载与命名空间校验。

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// variableNamePattern 对应 VARIABLE_NAME_PATTERN：^[^{}\s=]+$
	variableNamePattern = regexp.MustCompile(`^[^{}\s=]+$`)
	// variablePattern 对应 VARIABLE_PATTERN：\{\{\s*([^{}\s]+)\s*\}\}
	variablePattern = regexp.MustCompile(`\{\{\s*([^{}\s]+)\s*\}\}`)
	// fullVariablePattern 对应 FULL_VARIABLE_PATTERN
	fullVariablePattern = regexp.MustCompile(`^\{\{\s*([^{}\s]+)\s*\}\}$`)
	// removedDollarVariablePattern 对应 REMOVED_DOLLAR_VARIABLE_PATTERN
	removedDollarVariablePattern = regexp.MustCompile(`\$\{[^{}\s]+\}`)
	// reservedVariableNamespaces 对应 RESERVED_VARIABLE_NAMESPACES
	reservedVariableNamespaces = map[string]bool{"random": true}
)

// ValidateVariableNamespaces 校验变量名不使用保留命名空间且格式合法。
func ValidateVariableNamespaces(variables map[string]any) error {
	for name := range variables {
		if err := validateVariableName(name); err != nil {
			return err
		}
		if isReservedVariableNamespace(name) {
			return fmt.Errorf("变量名不允许使用保留命名空间: %s", name)
		}
	}
	return nil
}

func validateVariableName(name string) error {
	name = strings.TrimSpace(name)
	if !variableNamePattern.MatchString(name) {
		return fmt.Errorf("非法变量名: %s", name)
	}
	return nil
}

func validateRuntimeVariableName(name string) error {
	if err := validateVariableName(name); err != nil {
		return err
	}
	if isReservedVariableNamespace(name) {
		return fmt.Errorf("捕获变量不允许使用保留命名空间: %s", name)
	}
	return nil
}

func isReservedVariableNamespace(name string) bool {
	root := name
	if idx := strings.IndexAny(name, "./"); idx > 0 {
		root = name[:idx]
	}
	return reservedVariableNamespaces[root]
}

// ResolveVariables 解析 payload 中的 {{name}} 占位符（深度遍历 map/slice/string）。
// runtimeVariables 声明了执行期才解析的占位符，解析时保持原样。
func ResolveVariables(payload any, variables map[string]any, runtimeVariables map[string]bool) any {
	if variables == nil {
		variables = map[string]any{}
	}
	return resolveValue(payload, variables, runtimeVariables)
}

func resolveValue(value any, variables map[string]any, delayed map[string]bool) any {
	switch typed := value.(type) {
	case map[string]any:
		resolved := make(map[string]any, len(typed))
		for key, item := range typed {
			resolved[key] = resolveValue(item, variables, delayed)
		}
		return resolved
	case []any:
		resolved := make([]any, len(typed))
		for idx, item := range typed {
			resolved[idx] = resolveValue(item, variables, delayed)
		}
		return resolved
	case string:
		if removedDollarVariablePattern.MatchString(typed) {
			// ${name} 语法已移除，保留占位符会执行失败，直接原样返回由用户排查
			return typed
		}
		return resolveString(typed, variables, delayed)
	default:
		return value
	}
}

func resolveString(text string, variables map[string]any, delayed map[string]bool) any {
	if fullVariablePattern.MatchString(text) {
		name := fullVariablePattern.FindStringSubmatch(text)[1]
		if delayed[name] {
			return text
		}
		if value, ok := variables[name]; ok {
			return value
		}
		return text
	}
	return variablePattern.ReplaceAllStringFunc(text, func(match string) string {
		name := variablePattern.FindStringSubmatch(match)[1]
		if delayed[name] {
			return match
		}
		if value, ok := variables[name]; ok {
			return fmt.Sprintf("%v", value)
		}
		return match
	})
}

// ParseCLIVariables 对应 parse_cli_variables：--var KEY=VALUE。
func ParseCLIVariables(assignments []string) (map[string]any, error) {
	variables := map[string]any{}
	for _, assignment := range assignments {
		key, value, found := strings.Cut(assignment, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, fmt.Errorf("--var 必须使用 KEY=VALUE 格式: %s", assignment)
		}
		setVariable(variables, key, value)
	}
	if err := ValidateVariableNamespaces(variables); err != nil {
		return nil, err
	}
	return variables, nil
}

func setVariable(variables map[string]any, key string, value any) {
	// 支持点号路径，如 project.code=xxx
	if idx := strings.Index(key, "."); idx > 0 {
		head := key[:idx]
		child, ok := variables[head].(map[string]any)
		if !ok {
			child = map[string]any{}
			variables[head] = child
		}
		setVariable(child, key[idx+1:], value)
		return
	}
	variables[key] = value
}

// LoadVariablesFile 对应 load_variables_file：外部变量 YAML 文件。
func LoadVariablesFile(path string) (map[string]any, error) {
	filePath := expandHome(path)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("变量文件不存在: %s", filePath)
	}
	var payload map[string]any
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("变量文件必须是合法 YAML: %s", filePath)
	}
	rawVariablesAny, hasVariablesKey := payload["variables"]
	if !hasVariablesKey {
		rawVariablesAny = any(payload)
	}
	variables, ok := rawVariablesAny.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("变量文件 variables 必须是对象: %s", filePath)
	}
	if err := ValidateVariableNamespaces(variables); err != nil {
		return nil, err
	}
	return variables, nil
}

// MergeVariables 依序合并多个变量表，后者覆盖前者。
func MergeVariables(items ...map[string]any) map[string]any {
	merged := map[string]any{}
	for _, item := range items {
		if item == nil {
			continue
		}
		for key, value := range item {
			merged[key] = value
		}
	}
	return merged
}

// VariableKeys 返回变量表中全部变量名（含嵌套路径展开）。
func VariableKeys(variables map[string]any) []string {
	keys := []string{}
	collectVariableKeys(variables, "", &keys)
	return keys
}

func collectVariableKeys(variables map[string]any, prefix string, keys *[]string) {
	for key, value := range variables {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		if child, ok := value.(map[string]any); ok {
			collectVariableKeys(child, full, keys)
			continue
		}
		*keys = append(*keys, full)
	}
}
