package browser

// dsl.go — ydf_browser/dsl.py 的 Go 移植。
// 负责 YAML DSL 文档加载、步骤归一化与语法校验（目标属性白名单、
// 失败策略、BPR 录制配对、import 直提断言、变量解析等）。

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var validFailurePolicies = map[string]bool{
	"continue":   true,
	"stop_suite": true,
	"stop_run":   true,
}

var legacyActions = map[string]bool{
	"open_row_detail": true,
	"click_button":    true,
	"fill_field":      true,
	"fill_grid_row":   true,
	"assert_field":    true,
	"assert_text":     true,
	"save":            true,
	"submit":          true,
}

var openMenuStepKeys = map[string]bool{"action": true, "menu_path": true}
var componentStepKeys = map[string]bool{"component": true, "operation": true, "target": true}

// targetSchema 对应 Python 版 TargetSchema：允许的 target 属性 + 正确写法示例。
type targetSchema struct {
	allowedKeys map[string]bool
	example     string
}

func schema(example string, keys ...string) targetSchema {
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		allowed[key] = true
	}
	return targetSchema{allowedKeys: allowed, example: example}
}

// componentTargetSchemas 对应 Python 版 COMPONENT_TARGET_SCHEMAS（完整移植）。
var componentTargetSchemas = map[[2]string]targetSchema{
	{"action", "click"}: schema(`  - component: action
    operation: click
    target:
      fieldid: pageHeader_return`, "fieldid", "label"),
	{"amount_unit", "set"}: schema(`  - component: amount_unit
    operation: set
    target:
      value: 元`, "value"),
	{"attachment", "upload"}: schema(`  - component: attachment
    operation: upload
    target:
      attachments:
        - type: 询价
          file: tmp/attachments/询价.xlsx`, "attachments", "file", "path", "type", "attachment_type", "name"),
	{"bpr", "start"}:   schema("  - component: bpr\n    operation: start\n    target:\n      name: 修改并取消", "name", "description"),
	{"bpr", "stop"}:    schema("  - component: bpr\n    operation: stop\n    target: {}", "name", "description"),
	{"dialog", "cancel"}: schema(`  - component: dialog
    operation: cancel
    target:
      label: 取消`, "label", "expect_download", "download", "restore_previous", "restore_template"),
	{"dialog", "click"}: schema(`  - component: dialog
    operation: click
    target:
      label: 确认
      expect_download: true`, "label", "expect_download", "download", "restore_previous", "restore_template"),
	{"dialog", "confirm"}: schema(`  - component: dialog
    operation: confirm
    target:
      label: 确定`, "label", "expect_download", "download", "restore_previous", "restore_template"),
	{"editable_table", "add_row"}: schema(`  - component: editable_table
    operation: add_row
    target:
      label: 新增
      table: 支出明细
      editor: form`, "label", "action", "table", "name", "table_name", "editor"),
	{"editable_table", "fill_row"}: schema(`  - component: editable_table
    operation: fill_row
    target:
      table: 支出明细
      row_number: -1
      fields:
        支出明细: Codex支出明细
        数量: "1"`, "table", "name", "table_name", "row_number", "row_index", "fields", "values", "field", "equals"),
	{"form", "assert_values"}: schema(`  - component: form
    operation: assert_values
    target:
      fields:
        项目名称: puqh测试项目1`, "fields", "values", "field", "equals"),
	{"form", "capture_values"}: schema(`  - component: form
    operation: capture_values
    target:
      capture:
        tel: 手机号`, "capture"),
	{"form", "fill"}: schema(`  - component: form
    operation: fill
    target:
      fields:
        项目名称: puqh测试项目1`, "fields", "values", "field", "equals"),
	{"import", "download_template"}: schema(`  - component: import
    operation: download_template
    target:
      label: 下载模板`, "label", "template", "template_name", "name"),
	{"import", "upload"}: schema(`  - component: import
    operation: upload
    target:
      file: tmp/import.xlsx`, "label", "fieldid", "file", "files", "path", "assert"),
	{"notification", "assert"}: schema(`  - component: notification
    operation: assert
    target:
      type: error
      contains: 账户名称不能为空`, "type", "contains", "message_contains", "text_contains", "equals", "message"),
	{"notification", "close"}: schema(`  - component: notification
    operation: close
    target:
      type: error
      contains: 请联系管理员`, "type", "contains", "message_contains", "text_contains", "equals", "message", "label", "button"),
	{"workflow_result", "assert"}: schema(`  - component: workflow_result
    operation: assert
    target:
      total: 2
      succeeded: 1
      failed: 1`, "total", "succeeded", "failed", "title", "message", "contains"),
	{"workflow_result", "details"}: schema("  - component: workflow_result\n    operation: details\n    target: {}", "label"),
	{"workflow_result", "confirm"}: schema("  - component: workflow_result\n    operation: confirm\n    target: {}", "label"),
	{"popup_reference", "select"}: schema(`  - component: popup_reference
    operation: select
    target:
      title: 单位遴选
      value: 101001 江西省局本级（主管单位/单位经办）`, "title", "label", "name", "filter", "value", "values", "match", "matches", "selection", "selections", "row_number", "row_numbers", "row_index", "rows", "first_available", "select_first_available", "table", "table_name", "tab", "tabs", "tree", "trees", "confirm", "confirm_label"),
	{"query", "search"}: schema(`  - component: query
    operation: search
    target:
      conditions:
        - field: 项目负责人
          equals: 吕晓雷`, "conditions", "button"),
	{"query", "reset"}: schema("  - component: query\n    operation: reset\n    target: {}"),
	{"tab", "click"}: schema(`  - component: tab
    operation: click
    target:
      label: 全部`, "label"),
	{"table", "assert_column_values"}: schema(`  - component: table
    operation: assert_column_values
    target:
      column: 项目负责人
      equals: 吕晓雷`, "table", "table_name", "column", "equals"),
	{"table", "assert_columns"}: schema(`  - component: table
    operation: assert_columns
    target:
      table: 三级模板明细
      present:
        - 一级绩效指标
      absent:
        - 一级绩效指标分值`, "table", "name", "table_name", "present", "absent"),
	{"table", "assert_empty"}: schema(`  - component: table
    operation: assert_empty
    target:
      column: 项目名称`, "column"),
	{"table", "capture_row_values"}: schema(`  - component: table
    operation: capture_row_values
    target:
      table: 明细表
      match:
        column: 项目名称
        equals: "ys测试{{random.suffix}}"
      capture:
        yjxm.project_code: 项目代码`, "table", "table_name", "match", "capture"),
	{"table", "click_cell_link"}: schema(`  - component: table
    operation: click_cell_link
    target:
      match:
        column: 序号
        equals: 1
      column: 任务编码
      link_text: "202600000000060"`, "table", "table_name", "match", "row_number", "row_index", "first_available", "select_first_available", "column", "link_text"),
	{"table", "assert_row_values"}: schema(`  - component: table
    operation: assert_row_values
    target:
      match:
        column: 项目名称
        equals: puqh测试项目1
      fields:
        项目申报部门: 003 财务处`, "table", "table_name", "match", "fields", "values", "field", "equals"),
	{"table_row_action", "click"}: schema(`  - component: table_row_action
    operation: click
    target:
      action: 详情
      match:
        conditions:
          - column: 项目名称
            equals: puqh测试项目1`, "action", "table", "table_name", "match", "row_number", "row_numbers", "row_index", "first_available", "select_first_available"),
	{"table_row_selection", "select"}: schema(`  - component: table_row_selection
    operation: select
    target:
      table: 消费明细
      row_number: 1`, "table", "match", "row_number", "row_numbers", "row_index", "rows", "first_available", "select_first_available", "select_all"),
	{"toolbar", "click"}: schema(`  - component: toolbar
    operation: click
    target:
      label: 新增`, "label", "sub_label", "sub_action", "child_label", "child_action", "item", "option", "restore_previous", "restore_template"),
	{"transfer_dialog", "select"}: schema(`  - component: transfer_dialog
    operation: select
    target:
      title: 支付任务类型配置
      left:
        matches:
          - column: 任务名称
            equals: 示例任务`, "title", "label", "name", "move", "direction", "confirm", "items", "targets", "matches", "match", "left", "source", "equals", "column"),
	{"tree", "select"}: schema(`  - component: tree
    operation: select
    target:
      label: 根节点`, "label", "labels", "values", "node", "name", "component_name", "component_label", "region_label"),
}

var validComponentOperations = componentTargetSchemas

// LoadDslDocumentFromFile 对应 load_dsl_document_from_file。
func LoadDslDocumentFromFile(path string, variables map[string]any) (*DslDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("DSL 文件不存在: %s", path)
	}
	var payload any
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("DSL 文件必须是合法 YAML: %s", path)
	}
	return LoadDslDocument(payload, variables)
}

// LoadDslDocument 对应 load_dsl_document：解析 suite + cases 文档。
func LoadDslDocument(payload any, variables map[string]any) (*DslDocument, error) {
	if variables == nil {
		variables = map[string]any{}
	}
	if err := ValidateVariableNamespaces(variables); err != nil {
		return nil, err
	}
	runtimeVariables, err := collectRuntimeCaptureVariables(payload, variables)
	if err != nil {
		return nil, err
	}
	delayed := map[string]bool{}
	for _, name := range runtimeVariables {
		delayed[name] = true
	}
	payload = ResolveVariables(payload, variables, delayed)
	payloadMap, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("旧式 steps DSL 已废弃，请使用 suite + cases + steps 格式")
	}
	if _, hasCases := payloadMap["cases"]; !hasCases {
		return nil, fmt.Errorf("旧式 steps DSL 已废弃，请使用 suite + cases + steps 格式")
	}
	document, err := loadSuiteDocument(payloadMap)
	if err != nil {
		return nil, err
	}
	document.Suite.RuntimeVariables = runtimeVariables
	document.Suite.RequiresRuntimeSerial = len(runtimeVariables) > 0
	return document, nil
}

func loadSuiteDocument(payload map[string]any) (*DslDocument, error) {
	var err error
	suitePayload, ok := payload["suite"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("测试集 suite 必须是对象")
	}
	casesPayload, ok := payload["cases"].([]any)
	if !ok || len(casesPayload) == 0 {
		return nil, fmt.Errorf("测试集 cases 必须是非空列表")
	}
	if err := rejectDeprecatedSuiteContextFields(suitePayload); err != nil {
		return nil, err
	}
	suite := DslTestSuite{
		Description:   textOf(suitePayload["description"]),
		Tenant:        textOf(suitePayload["tenant"]),
		DefaultOrg:    textOf(suitePayload["default_org"]),
		FiscalYear:    textOf(suitePayload["fiscal_year"]),
		OnCaseFailure: "continue",
	}
	suite.Code, err = requiredText(suitePayload, "code", "suite.code")
	if err != nil {
		return nil, err
	}
	suite.Name, err = requiredText(suitePayload, "name", "suite.name")
	if err != nil {
		return nil, err
	}
	policy := textOf(suitePayload["on_case_failure"])
	if policy == "" {
		policy = textOf(suitePayload["failure_strategy"])
	}
	if policy == "" {
		policy = textOf(suitePayload["on_failure"])
	}
	suite.OnCaseFailure, err = normalizeFailurePolicy(policy, "suite.on_case_failure")
	if err != nil {
		return nil, err
	}

	cases := make([]DslTestCase, 0, len(casesPayload))
	for index, item := range casesPayload {
		cse, err := loadCase(item, index+1, suite.DefaultOrg)
		if err != nil {
			return nil, err
		}
		cases = append(cases, *cse)
	}
	if suite.Tenant == "" {
		return nil, fmt.Errorf("suite.tenant 必填")
	}
	return &DslDocument{Suite: suite, Cases: cases}, nil
}

func loadCase(payload any, index int, suiteDefaultOrg string) (*DslTestCase, error) {
	casePayload, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("测试用例必须是对象: cases[%d]", index)
	}
	if err := rejectDeprecatedCaseContextFields(casePayload, index); err != nil {
		return nil, err
	}
	rawSteps, err := extractSteps(casePayload)
	if err != nil {
		return nil, fmt.Errorf("cases[%d]: %w", index, err)
	}
	steps := make([]ActionStep, 0, len(rawSteps))
	for _, item := range rawSteps {
		step, stepErr := NormalizeStep(item)
		if stepErr != nil {
			return nil, fmt.Errorf("cases[%d]: %w", index, stepErr)
		}
		steps = append(steps, *step)
	}
	if err := validateSteps(steps, fmt.Sprintf("cases[%d].steps", index)); err != nil {
		return nil, err
	}
	caseDefaultOrg := textOf(casePayload["default_org"])
	if caseDefaultOrg == "" {
		caseDefaultOrg = suiteDefaultOrg
	}
	if caseDefaultOrg == "" {
		return nil, fmt.Errorf("suite.default_org 或 cases[%d].default_org 必填", index)
	}
	caseCode, err := requiredText(casePayload, "code", fmt.Sprintf("cases[%d].code", index))
	if err != nil {
		return nil, err
	}
	caseName, err := requiredText(casePayload, "name", fmt.Sprintf("cases[%d].name", index))
	if err != nil {
		return nil, err
	}
	tags, err := loadTags(casePayload["tags"], index)
	if err != nil {
		return nil, err
	}
	return &DslTestCase{
		Code:        caseCode,
		Name:        caseName,
		Description: textOf(casePayload["description"]),
		User:        textOf(casePayload["user"]),
		DefaultOrg:  caseDefaultOrg,
		Tags:        tags,
		Steps:       steps,
	}, nil
}

func extractSteps(payload map[string]any) ([]any, error) {
	rawSteps, ok := payload["steps"].([]any)
	if !ok {
		rawSteps, ok = payload["actions"].([]any)
	}
	if !ok || rawSteps == nil {
		return nil, fmt.Errorf("DSL 必须是列表或包含 steps 的对象")
	}
	return rawSteps, nil
}

func rejectDeprecatedSuiteContextFields(payload map[string]any) error {
	if _, ok := payload["username"]; ok {
		return fmt.Errorf("suite.username/password 已废弃，请只在 cases[].user 配置用户别名，真实用户名和密码只允许放在 env.users")
	}
	if _, ok := payload["password"]; ok {
		return fmt.Errorf("suite.username/password 已废弃，请只在 cases[].user 配置用户别名，真实用户名和密码只允许放在 env.users")
	}
	deprecated := [][2]string{
		{"tenant_name", "suite.tenant_name 已废弃，请使用 suite.tenant"},
		{"tenantName", "suite.tenantName 已废弃，请使用 suite.tenant"},
		{"default_org_code", "suite.default_org_code 已废弃，请使用 suite.default_org"},
		{"default_org_name", "suite.default_org_name 已废弃，请使用 suite.default_org"},
		{"defaultOrg", "suite.defaultOrg 已废弃，请使用 suite.default_org"},
		{"defaultOrgCode", "suite.defaultOrgCode 已废弃，请使用 suite.default_org"},
		{"defaultOrgName", "suite.defaultOrgName 已废弃，请使用 suite.default_org"},
	}
	for _, pair := range deprecated {
		if _, ok := payload[pair[0]]; ok {
			return fmt.Errorf("%s", pair[1])
		}
	}
	return nil
}

func rejectDeprecatedCaseContextFields(payload map[string]any, index int) error {
	if _, ok := payload["username"]; ok {
		return fmt.Errorf("cases[%d].username/password 已废弃，请只配置 user 别名，真实用户名和密码只允许放在 env.users", index)
	}
	if _, ok := payload["password"]; ok {
		return fmt.Errorf("cases[%d].username/password 已废弃，请只配置 user 别名，真实用户名和密码只允许放在 env.users", index)
	}
	deprecated := [][2]string{
		{"tenant", fmt.Sprintf("cases[%d].tenant 已废弃，租户只能配置在 suite.tenant", index)},
		{"tenant_name", fmt.Sprintf("cases[%d].tenant_name 已废弃，租户只能配置在 suite.tenant", index)},
		{"tenantName", fmt.Sprintf("cases[%d].tenantName 已废弃，租户只能配置在 suite.tenant", index)},
		{"default_org_code", fmt.Sprintf("cases[%d].default_org_code 已废弃，请使用 cases[%d].default_org", index, index)},
		{"default_org_name", fmt.Sprintf("cases[%d].default_org_name 已废弃，请使用 cases[%d].default_org", index, index)},
		{"defaultOrg", fmt.Sprintf("cases[%d].defaultOrg 已废弃，请使用 cases[%d].default_org", index, index)},
		{"defaultOrgCode", fmt.Sprintf("cases[%d].defaultOrgCode 已废弃，请使用 cases[%d].default_org", index, index)},
		{"defaultOrgName", fmt.Sprintf("cases[%d].defaultOrgName 已废弃，请使用 cases[%d].default_org", index, index)},
	}
	for _, pair := range deprecated {
		if _, ok := payload[pair[0]]; ok {
			return fmt.Errorf("%s", pair[1])
		}
	}
	return nil
}

func requiredText(payload map[string]any, key, label string) (string, error) {
	value := textOf(payload[key])
	if value == "" {
		return "", fmt.Errorf("测试集 DSL 缺少必填字段: %s", label)
	}
	return value, nil
}

func normalizeFailurePolicy(value any, label string) (string, error) {
	text := strings.ReplaceAll(textOf(value), "-", "_")
	if text == "skip" || text == "continue_suite" {
		text = "continue"
	}
	if text == "" {
		text = "continue"
	}
	if !validFailurePolicies[text] {
		return "", fmt.Errorf("不支持的失败策略: %s=%v", label, value)
	}
	return text, nil
}

func loadTags(value any, index int) ([]string, error) {
	if value == nil {
		return []string{}, nil
	}
	if text, ok := value.(string); ok {
		tags := []string{}
		for _, item := range strings.Split(text, ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				tags = append(tags, trimmed)
			}
		}
		return tags, nil
	}
	if items, ok := value.([]any); ok {
		tags := []string{}
		for _, item := range items {
			if trimmed := textOf(item); trimmed != "" {
				tags = append(tags, trimmed)
			}
		}
		return tags, nil
	}
	return nil, fmt.Errorf("测试用例 tags 必须是列表或逗号分隔字符串: cases[%d].tags", index)
}

// NormalizeStep 对应 normalize_step：页面级步骤（action）与组件步骤（component/operation/target）。
func NormalizeStep(item any) (*ActionStep, error) {
	stepMap, ok := item.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("非法 DSL step: %v", item)
	}
	if _, hasAction := stepMap["action"]; hasAction {
		action := textOf(stepMap["action"])
		if legacyActions[action] {
			return nil, fmt.Errorf("旧 DSL 已移除，请改用组件语义 DSL: %s", action)
		}
		if action != "open_menu" {
			return nil, fmt.Errorf("不支持的页面级 DSL 动作: %s", action)
		}
		params := map[string]any{}
		for key, value := range stepMap {
			if key != "action" && openMenuStepKeys[key] {
				params[key] = value
			}
		}
		return &ActionStep{
			Kind:      "page",
			Action:    action,
			Params:    params,
			ExtraKeys: unsupportedKeys(stepMap, openMenuStepKeys),
		}, nil
	}
	if _, hasComponent := stepMap["component"]; hasComponent {
		if _, hasOperation := stepMap["operation"]; !hasOperation {
			return nil, fmt.Errorf("非法 DSL step: %v", item)
		}
		if _, hasTarget := stepMap["target"]; !hasTarget {
			return nil, fmt.Errorf("非法 DSL step: %v", item)
		}
		target, ok := stepMap["target"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("组件 DSL 的 target 必须是对象: %v", item)
		}
		return &ActionStep{
			Kind:      "component",
			Component: textOf(stepMap["component"]),
			Operation: textOf(stepMap["operation"]),
			Target:    target,
			ExtraKeys: unsupportedKeys(stepMap, componentStepKeys),
		}, nil
	}
	return nil, fmt.Errorf("非法 DSL step: %v", item)
}

func unsupportedKeys(step map[string]any, allowed map[string]bool) []string {
	keys := []string{}
	for key := range step {
		if !allowed[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func validateSteps(steps []ActionStep, path string) error {
	errors := []string{}
	for index, step := range steps {
		stepPath := fmt.Sprintf("%s[%d]", path, index+1)
		errors = append(errors, validateStep(step, stepPath)...)
	}
	errors = append(errors, validateBPRState(steps, path)...)
	errors = append(errors, validateSubmittedImportAssertions(steps, path)...)
	if len(errors) > 0 {
		return fmt.Errorf("DSL 语法校验失败:\n- %s", strings.Join(errors, "\n- "))
	}
	return nil
}

func validateStep(step ActionStep, path string) []string {
	errors := []string{}
	if len(step.ExtraKeys) > 0 {
		keys := strings.Join(step.ExtraKeys, ", ")
		if step.Kind == "page" {
			errors = append(errors, fmt.Sprintf("%s: %s 不支持属性: %s", path, step.Action, keys))
		} else {
			label := step.Component + "." + step.Operation
			if _, known := componentTargetSchemas[[2]string{step.Component, step.Operation}]; known {
				errors = append(errors, fmt.Sprintf("%s: %s 的 target 不支持属性: %s", path, label, keys))
			} else {
				errors = append(errors, fmt.Sprintf("%s: %s 不支持属性: %s", path, label, keys))
			}
		}
	}
	if step.Kind == "page" {
		if step.Action == "open_menu" && textOf(step.Params["menu_path"]) == "" {
			errors = append(errors, fmt.Sprintf("%s: open_menu 缺少 menu_path", path))
		}
		return errors
	}
	if step.Kind != "component" {
		errors = append(errors, fmt.Sprintf("%s: 不支持的 DSL step 类型: %s", path, step.Kind))
		return errors
	}
	label := step.Component + "." + step.Operation
	schemaDef, known := componentTargetSchemas[[2]string{step.Component, step.Operation}]
	if !known {
		errors = append(errors, fmt.Sprintf("%s: 不支持的组件操作: %s", path, label))
		return errors
	}
	for key := range step.Target {
		if !schemaDef.allowedKeys[key] {
			errors = append(errors, fmt.Sprintf("%s: %s 的 target 不支持属性: %s\n正确写法示例:\n%s", path, label, key, schemaDef.example))
		}
	}
	return errors
}

func validateBPRState(steps []ActionStep, path string) []string {
	errors := []string{}
	activeStartIndex := -1
	for index, step := range steps {
		if step.Kind != "component" || step.Component != "bpr" {
			continue
		}
		switch step.Operation {
		case "start":
			if activeStartIndex >= 0 {
				errors = append(errors, fmt.Sprintf("%s[%d]: 不支持嵌套 BPR 录制，请先为 %s[%d] 补充 bpr.stop", path, index+1, path, activeStartIndex+1))
				continue
			}
			activeStartIndex = index
		case "stop":
			if activeStartIndex < 0 {
				errors = append(errors, fmt.Sprintf("%s[%d]: 没有正在录制的 BPR session，不能执行 bpr.stop", path, index+1))
				continue
			}
			activeStartIndex = -1
		}
	}
	if activeStartIndex >= 0 {
		errors = append(errors, fmt.Sprintf("%s[%d]: BPR 录制缺少 bpr.stop", path, activeStartIndex+1))
	}
	return errors
}

func validateSubmittedImportAssertions(steps []ActionStep, path string) []string {
	errors := []string{}
	for index, step := range steps {
		if !isSubmittedImportStep(step) {
			continue
		}
		stepPath := fmt.Sprintf("%s[%d]", path, index+1)
		if index == len(steps)-1 {
			errors = append(errors, fmt.Sprintf("%s: `assert.status: submitted` 不能是用例最后一步，必须增加后置业务断言", stepPath))
			continue
		}
		hasAssertion := false
		for _, later := range steps[index+1:] {
			if isBusinessAssertionStep(later) {
				hasAssertion = true
				break
			}
		}
		if !hasAssertion {
			errors = append(errors, fmt.Sprintf("%s: `assert.status: submitted` 后续必须包含业务断言，例如 table.assert_row_values、form.assert_values 或 notification.assert", stepPath))
		}
	}
	return errors
}

func isSubmittedImportStep(step ActionStep) bool {
	assertion, ok := step.Target["assert"].(map[string]any)
	if !ok {
		return false
	}
	return step.Kind == "component" &&
		step.Component == "import" &&
		step.Operation == "upload" &&
		strings.EqualFold(textOf(assertion["status"]), "submitted")
}

func isBusinessAssertionStep(step ActionStep) bool {
	if step.Kind != "component" {
		return false
	}
	switch [2]string{step.Component, step.Operation} {
	case [2]string{"form", "assert_values"},
		[2]string{"notification", "assert"},
		[2]string{"workflow_result", "assert"},
		[2]string{"table", "assert_column_values"},
		[2]string{"table", "assert_columns"},
		[2]string{"table", "assert_empty"},
		[2]string{"table", "assert_row_values"}:
		return true
	}
	return false
}

func collectRuntimeCaptureVariables(payload any, externalVariables map[string]any) ([]string, error) {
	names := []string{}
	payloadMap, ok := payload.(map[string]any)
	if !ok {
		return names, nil
	}
	cases, ok := payloadMap["cases"].([]any)
	if !ok {
		return names, nil
	}
	for _, caseAny := range cases {
		casePayload, ok := caseAny.(map[string]any)
		if !ok {
			continue
		}
		steps, ok := casePayload["steps"].([]any)
		if !ok {
			steps, _ = casePayload["actions"].([]any)
		}
		for _, stepAny := range steps {
			stepMap, ok := stepAny.(map[string]any)
			if !ok || !isRuntimeCaptureStep(stepMap) {
				continue
			}
			target, _ := stepMap["target"].(map[string]any)
			capture, ok := target["capture"].(map[string]any)
			if !ok {
				continue
			}
			for rawName := range capture {
				name := textOf(rawName)
				if err := validateRuntimeVariableName(name); err != nil {
					return nil, err
				}
				if variablePathConflicts(externalVariables, name) {
					return nil, fmt.Errorf("捕获变量不能与外部变量重名: %s", name)
				}
				if !containsString(names, name) {
					names = append(names, name)
				}
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

func isRuntimeCaptureStep(step map[string]any) bool {
	component := textOf(step["component"])
	operation := textOf(step["operation"])
	return [2]string{component, operation} == [2]string{"table", "capture_row_values"} ||
		[2]string{component, operation} == [2]string{"form", "capture_values"}
}

func variablePathConflicts(variables map[string]any, path string) bool {
	parts := strings.Split(path, ".")
	current := variables
	for idx, part := range parts {
		value, ok := current[part]
		if !ok {
			return false
		}
		if idx == len(parts)-1 {
			return true
		}
		child, ok := value.(map[string]any)
		if !ok {
			return true
		}
		current = child
	}
	return false
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
