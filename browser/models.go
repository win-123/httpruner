// Package browser embeds ydf-browser (YonDiF 单据页浏览器自动化) into hrp.
//
// ydf-browser 原为独立 Python CLI（Playwright 驱动，YAML DSL 描述业务步骤）。
// 本包将其数据层（DSL 模型/加载/校验、环境配置、变量解析）用 Go 忠实重写，
// 浏览器执行层通过 Engine 接口抽象：当前默认对接已安装的 ydf-browser runtime，
// 后续可无缝替换为基于 hrp uixt 的原生 Go 引擎。
//
// 命令入口：hrp browser（run / validate / doctor / login / menu ...）
package browser

// ActionStep 对应 Python 版 models.ActionStep。
// Kind 为 "page"（页面级动作，如 open_menu）或 "component"（组件语义步骤）。
type ActionStep struct {
	Kind      string         `json:"kind"`
	Action    string         `json:"action,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
	Component string         `json:"component,omitempty"`
	Operation string         `json:"operation,omitempty"`
	Target    map[string]any `json:"target,omitempty"`
	ExtraKeys []string       `json:"extra_keys,omitempty"`
}

// DslTestCase 对应 Python 版 models.DslTestCase。
type DslTestCase struct {
	Code        string       `json:"code" yaml:"code"`
	Name        string       `json:"name" yaml:"name"`
	Description string       `json:"description" yaml:"description"`
	User        string       `json:"user" yaml:"user"`
	DefaultOrg  string       `json:"default_org" yaml:"default_org"`
	Tags        []string     `json:"tags" yaml:"tags"`
	Steps       []ActionStep `json:"steps" yaml:"steps"`
}

// DslTestSuite 对应 Python 版 models.DslTestSuite。
type DslTestSuite struct {
	Code                  string   `json:"code" yaml:"code"`
	Name                  string   `json:"name" yaml:"name"`
	Description           string   `json:"description" yaml:"description"`
	Tenant                string   `json:"tenant" yaml:"tenant"`
	DefaultOrg            string   `json:"default_org" yaml:"default_org"`
	FiscalYear            string   `json:"fiscal_year" yaml:"fiscal_year"`
	OnCaseFailure         string   `json:"on_case_failure" yaml:"on_case_failure"`
	RuntimeVariables      []string `json:"runtime_variables" yaml:"runtime_variables"`
	RequiresRuntimeSerial bool     `json:"requires_runtime_serial" yaml:"requires_runtime_serial"`
}

// DslDocument 对应 Python 版 models.DslDocument（suite + cases）。
type DslDocument struct {
	Suite DslTestSuite
	Cases []DslTestCase
}

// StepResult 对应 Python 版 models.StepResult（运行报告中的单步结果）。
type StepResult struct {
	Action  string         `json:"action"`
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

// ExecutionReport 对应 Python 版 models.ExecutionReport 的核心字段，
// 用于解析 ydf-browser runtime 输出的运行报告 JSON。
type ExecutionReport struct {
	Success     bool         `json:"success"`
	StepResults []StepResult `json:"step_results"`
	Errors      []string     `json:"errors"`
	Timing      struct {
		TotalMs float64 `json:"total_ms,omitempty"`
	} `json:"timing,omitempty"`
}
