package browser

// command.go — hrp browser 子命令。
//
// 三个层次的命令：
//   - validate：纯 Go 实现，对 DSL 文档做与 Python 版一致的语法校验（无需 runtime）
//   - run：先在 Go 侧完成 DSL 校验（fail fast），再交给 Engine 执行浏览器自动化
//   - 其他（doctor/login/menu/...）：原样透传给 Engine
//
// 这样 hrp 获得完整的 ydf-browser 能力入口，同时逐步向原生 Go 引擎迁移。

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// CmdBrowser 是 hrp browser 根命令。
var CmdBrowser = &cobra.Command{
	Use:                "browser",
	Short:              "YonDiF 单据页浏览器自动化（ydf-browser）",
	Long:               "使用 YAML DSL 描述 YonDiF 单据页业务步骤并执行，生成可追溯的运行报告。\n详细用法与 Python 版 ydf-browser CLI 保持一致。",
	DisableFlagParsing: true,
	SilenceUsage:       true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
			return cmd.Help()
		}
		switch args[0] {
		case "run", "validate":
			// 已注册为显式子命令，正常不会走到这里
			return cmd.Help()
		default:
			// doctor / login / menu / completion / update / profile 等原样透传
			engine, err := DefaultEngine()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "[ydf] engine: %s\n", engine.Name())
			return engine.Run(args)
		}
	},
}

func init() {
	CmdBrowser.AddCommand(browserRunCmd, browserValidateCmd)
}

var browserRunCmd = &cobra.Command{
	Use:                "run --dsl <path>... [flags]",
	Short:              "运行 ydf-browser DSL 测试集（Go 侧预校验，runtime 执行）",
	DisableFlagParsing: true,
	SilenceUsage:       true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateDSLArgs(args); err != nil {
			return err
		}
		engine, err := DefaultEngine()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "[ydf] DSL 预校验通过，engine: %s\n", engine.Name())
		return engine.Run(args)
	},
}

var browserValidateCmd = &cobra.Command{
	Use:                "validate --dsl <path>... [flags]",
	Short:              "校验 ydf-browser DSL 语法（纯 Go 实现，无需 runtime）",
	DisableFlagParsing: true,
	SilenceUsage:       true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runNativeValidate(cmd, args)
	},
}

// validateDSLArgs 对 run 的参数做 Go 侧 DSL 预校验；透传给 runtime 前快速失败。
func validateDSLArgs(args []string) error {
	dslPaths := flagValues(args, "--dsl")
	if len(dslPaths) == 0 {
		// 未显式给 --dsl 时交由 runtime 按自身默认行为处理（如目录扫描）
		return nil
	}
	variables, err := loadVariablesFromArgs(args)
	if err != nil {
		return err
	}
	for _, path := range dslPaths {
		if _, err := LoadDslDocumentFromFile(path, variables); err != nil {
			return fmt.Errorf("DSL 预校验失败: %w", err)
		}
	}
	return nil
}

// runNativeValidate 是 validate 子命令的纯 Go 实现。
func runNativeValidate(cmd *cobra.Command, args []string) error {
	dslPaths := flagValues(args, "--dsl")
	if len(dslPaths) == 0 {
		return fmt.Errorf("validate 需要至少一个 --dsl 参数")
	}
	variables, err := loadVariablesFromArgs(args)
	if err != nil {
		return err
	}
	filters := loadCaseFilters(args)
	totalCases := 0
	for _, path := range dslPaths {
		document, err := LoadDslDocumentFromFile(path, variables)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		cases := filterCases(document.Cases, filters)
		totalCases += len(cases)
		fmt.Fprintf(cmd.OutOrStdout(), "✔ %s: suite=%s(%s) tenant=%s cases=%d\n",
			filepath.Base(path), document.Suite.Code, document.Suite.Name, document.Suite.Tenant, len(cases))
		for _, cse := range cases {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s (steps=%d user=%s org=%s)\n",
				cse.Code, cse.Name, len(cse.Steps), orDefault(cse.User, "默认用户"), cse.DefaultOrg)
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "校验通过：%d 个 DSL 文档，共 %d 个用例\n", len(dslPaths), totalCases)
	return nil
}

// —— 参数解析辅助（支持 --flag value 与 --flag=value，可重复） ——

func flagValues(args []string, flag string) []string {
	values := []string{}
	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if arg == flag {
			if idx+1 < len(args) {
				values = append(values, args[idx+1])
				idx++
			}
			continue
		}
		if strings.HasPrefix(arg, flag+"=") {
			values = append(values, strings.TrimPrefix(arg, flag+"="))
		}
	}
	return values
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag || strings.HasPrefix(arg, flag+"=") {
			return true
		}
	}
	return false
}

func loadVariablesFromArgs(args []string) (map[string]any, error) {
	variables := map[string]any{}
	for _, path := range flagValues(args, "--vars-file") {
		fileVars, err := LoadVariablesFile(path)
		if err != nil {
			return nil, err
		}
		variables = MergeVariables(variables, fileVars)
	}
	cliVars, err := ParseCLIVariables(flagValues(args, "--var"))
	if err != nil {
		return nil, err
	}
	return MergeVariables(variables, cliVars), nil
}

// caseFilters 对应 run/validate 的用例筛选参数。
type caseFilters struct {
	caseCodes []string
	caseGlobs []string
	grep      string
	tags      []string
}

func loadCaseFilters(args []string) caseFilters {
	return caseFilters{
		caseCodes: flagValues(args, "--case"),
		caseGlobs: flagValues(args, "--case-glob"),
		grep:      firstFlagValue(args, "--grep"),
		tags:      flagValues(args, "--tag"),
	}
}

func firstFlagValue(args []string, flag string) string {
	values := flagValues(args, flag)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// filterCases 对应 runner.filter_cases 的核心逻辑。
func filterCases(cases []DslTestCase, filters caseFilters) []DslTestCase {
	if len(filters.caseCodes) == 0 && len(filters.caseGlobs) == 0 &&
		filters.grep == "" && len(filters.tags) == 0 {
		return cases
	}
	selected := []DslTestCase{}
	for _, cse := range cases {
		if len(filters.caseCodes) > 0 && !containsString(filters.caseCodes, cse.Code) {
			continue
		}
		if len(filters.caseGlobs) > 0 && !matchAnyGlob(filters.caseGlobs, cse.Code) {
			continue
		}
		if filters.grep != "" && !strings.Contains(cse.Name, filters.grep) &&
			!strings.Contains(caseContent(cse), filters.grep) {
			continue
		}
		if len(filters.tags) > 0 && !hasAnyTag(cse.Tags, filters.tags) {
			continue
		}
		selected = append(selected, cse)
	}
	return selected
}

func matchAnyGlob(patterns []string, code string) bool {
	for _, pattern := range patterns {
		if ok, err := filepath.Match(pattern, code); err == nil && ok {
			return true
		}
	}
	return false
}

func hasAnyTag(tags, wanted []string) bool {
	tagSet := map[string]bool{}
	for _, tag := range tags {
		tagSet[tag] = true
	}
	for _, tag := range wanted {
		if tagSet[tag] {
			return true
		}
	}
	return false
}

func caseContent(cse DslTestCase) string {
	var builder strings.Builder
	builder.WriteString(cse.Name)
	builder.WriteString("\n")
	for _, step := range cse.Steps {
		builder.WriteString(stepSummary(step))
		builder.WriteString("\n")
	}
	return builder.String()
}

func stepSummary(step ActionStep) string {
	if step.Kind == "page" {
		return step.Action + " " + fmt.Sprintf("%v", step.Params)
	}
	keys := make([]string, 0, len(step.Target))
	for key := range step.Target {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return fmt.Sprintf("%s.%s %v", step.Component, step.Operation, keys)
}
