package browser

// config.go — ydf_browser/config.py 的 Go 移植。
// 负责 env.yaml + env.local.yaml 的加载与合并、用户别名解析、端点默认值。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath 对应 default_config_path：~/.ydf-browser/config/env.yaml
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".ydf-browser/config/env.yaml"
	}
	return filepath.Join(home, ".ydf-browser", "config", "env.yaml")
}

// EndpointConfig 对应 Python 版 config.EndpointConfig，字段与默认值保持一致。
type EndpointConfig struct {
	ServiceTree      string `yaml:"service_tree"`
	ServiceDetail    string `yaml:"service_detail"`
	Templates        string `yaml:"templates"`
	TenantInformation string `yaml:"tenant_information"`
	SwitchTenant     string `yaml:"switch_tenant"`
	SwitchFiscalYear string `yaml:"switch_fiscal_year"`
	OrgTree          string `yaml:"org_tree"`
	UpdateDefaultOrg string `yaml:"update_default_org"`
	UserContext      string `yaml:"user_context"`
}

func defaultEndpointConfig() EndpointConfig {
	return EndpointConfig{
		ServiceTree:       "/iuap-apcom-workbench/menubar/getAllLight/v3",
		ServiceDetail:     "/iuap-apcom-workbench/service/getServiceInfoWithDetail",
		Templates:         "/yondif-ams-be/ytemplate/assign/matchingTemplate",
		TenantInformation: "/iuap-data-common/init/allInformation",
		SwitchTenant:      "/",
		SwitchFiscalYear:  "/yondif-ams-be/context/switch/fiscal/year",
		OrgTree:           "/fbdi-be/fbdi/bas/getOrgTree",
		UpdateDefaultOrg:  "/iuap-apcom-workbench/manager/globalPerference/updateUserDefaultOrg",
		UserContext:       "/iuap-apcom-workbench/manager/globalPerference/getUser",
	}
}

// UserCredential 对应 Python 版 config.UserCredential。
type UserCredential struct {
	Username string
	Password string
}

// EnvironmentConfig 对应 Python 版 config.EnvironmentConfig。
type EnvironmentConfig struct {
	EnvName        string
	BaseURL        string
	SysID          string
	Username       string
	Password       string
	Country        string
	TenantID       string
	LoginType      string
	Finger         string
	OrgID          string
	DefaultOrg     string
	DefaultOrgCode string
	DefaultOrgName string
	Endpoints      EndpointConfig
	Users          map[string]UserCredential
	BPR            *UserCredential
	Variables      map[string]any
}

// WithUser 对应 EnvironmentConfig.with_user：按别名切换账号。
func (env EnvironmentConfig) WithUser(user string) (EnvironmentConfig, error) {
	alias := strings.TrimSpace(user)
	if alias == "" {
		return env, nil
	}
	credential, ok := env.Users[alias]
	if !ok {
		available := make([]string, 0, len(env.Users))
		for name := range env.Users {
			available = append(available, name)
		}
		sort.Strings(available)
		return env, fmt.Errorf("用户别名 '%s' 不存在，可用用户: %s", alias, strings.Join(available, ", "))
	}
	env.Username = credential.Username
	env.Password = credential.Password
	return env, nil
}

type configLoader struct {
	ConfigPath     string
	LocalConfigPath string
	environments   map[string]EnvironmentConfig
}

// NewConfigLoader 对应 ConfigLoader.__init__：加载并合并 base 与 local 配置。
func NewConfigLoader(configPath string) (*configLoader, error) {
	if strings.TrimSpace(configPath) == "" {
		configPath = DefaultConfigPath()
	}
	configPath = expandHome(configPath)
	loader := &configLoader{
		ConfigPath:      configPath,
		LocalConfigPath: filepath.Join(filepath.Dir(configPath), "env.local.yaml"),
		environments:    map[string]EnvironmentConfig{},
	}
	if err := loader.load(); err != nil {
		return nil, err
	}
	return loader, nil
}

// GetEnv 对应 ConfigLoader.get_env。
func (l *configLoader) GetEnv(envName string) (EnvironmentConfig, error) {
	if envName == "" {
		envName = "ydf"
	}
	env, ok := l.environments[envName]
	if !ok {
		available := make([]string, 0, len(l.environments))
		for name := range l.environments {
			available = append(available, name)
		}
		sort.Strings(available)
		return env, fmt.Errorf("环境 '%s' 不存在，可用环境: %s", envName, strings.Join(available, ", "))
	}
	return env, nil
}

// Environments 返回全部环境名（已排序）。
func (l *configLoader) Environments() []string {
	names := make([]string, 0, len(l.environments))
	for name := range l.environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (l *configLoader) load() error {
	baseData, err := readYAMLMap(l.ConfigPath, true)
	if err != nil {
		return err
	}
	localData, err := readYAMLMap(l.LocalConfigPath, false)
	if err != nil {
		return err
	}
	baseEnvs, _ := baseData["environments"].(map[string]any)
	if baseEnvs == nil {
		baseEnvs = map[string]any{}
	}
	localEnvs, _ := localData["environments"].(map[string]any)
	if localEnvs == nil {
		localEnvs = map[string]any{}
	}
	merged := mergeEnvironments(baseEnvs, localEnvs)
	for name, envDataAny := range merged {
		envData, _ := envDataAny.(map[string]any)
		if envData == nil {
			envData = map[string]any{}
		}
		env, err := parseEnvironment(name, envData)
		if err != nil {
			return err
		}
		l.environments[name] = env
	}
	return nil
}

func parseEnvironment(name string, envData map[string]any) (EnvironmentConfig, error) {
	endpoints := defaultEndpointConfig()
	if raw, ok := envData["endpoints"].(map[string]any); ok {
		applyYAMLOverride(&endpoints, raw)
	}
	defaultUsername := textOf(envData["username"])
	defaultPassword := textOf(envData["password"])

	users := map[string]UserCredential{}
	hasExplicitDefaultUser := false
	if rawUsers, ok := envData["users"].(map[string]any); ok {
		for alias, credentialAny := range rawUsers {
			credentialData, ok := credentialAny.(map[string]any)
			if !ok {
				return EnvironmentConfig{}, fmt.Errorf("用户配置必须是对象: %s", alias)
			}
			credential := UserCredential{
				Username: orDefault(textOf(credentialData["username"]), defaultUsername),
				Password: orDefault(textOf(credentialData["password"]), defaultPassword),
			}
			users[alias] = credential
			if alias == "默认用户" {
				hasExplicitDefaultUser = true
			}
		}
	}
	if !hasExplicitDefaultUser && defaultUsername != "" {
		// 对应 _should_synthesize_default_user：未显式配置「默认用户」时自动合成
		users["默认用户"] = UserCredential{Username: defaultUsername, Password: defaultPassword}
	}

	var bpr *UserCredential
	if raw, ok := envData["bpr"].(map[string]any); ok {
		bpr = &UserCredential{
			Username: textOf(raw["username"]),
			Password: textOf(raw["password"]),
		}
	}

	variables := map[string]any{}
	if raw, ok := envData["variables"].(map[string]any); ok {
		variables = raw
	}
	if err := ValidateVariableNamespaces(variables); err != nil {
		return EnvironmentConfig{}, fmt.Errorf("环境 %s: %w", name, err)
	}

	return EnvironmentConfig{
		EnvName:        name,
		BaseURL:        textOf(envData["base_url"]),
		SysID:          orDefault(textOf(envData["sysid"]), "yonbip"),
		Username:       defaultUsername,
		Password:       defaultPassword,
		Country:        orDefault(textOf(envData["country"]), "86"),
		TenantID:       orDefault(textOf(envData["tenantid"]), "-1"),
		LoginType:      orDefault(textOf(envData["login_type"]), "normal"),
		Finger:         orDefault(textOf(envData["finger"]), "ba4a682bd197d088af365452c56b2bf2"),
		OrgID:          orDefault(textOf(envData["org_id"]), "666666"),
		DefaultOrg:     textOf(envData["default_org"]),
		DefaultOrgCode: textOf(envData["default_org_code"]),
		DefaultOrgName: textOf(envData["default_org_name"]),
		Endpoints:      endpoints,
		Users:          users,
		BPR:            bpr,
		Variables:      variables,
	}, nil
}

func mergeEnvironments(base, local map[string]any) map[string]any {
	merged := map[string]any{}
	for name := range base {
		merged[name] = base[name]
	}
	for name := range local {
		merged[name] = deepMerge(asMap(merged[name]), asMap(local[name]))
	}
	return merged
}

func deepMerge(base, override map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range base {
		result[key] = value
	}
	for key, value := range override {
		if overrideMap, ok := value.(map[string]any); ok {
			if baseMap, ok := result[key].(map[string]any); ok {
				result[key] = deepMerge(baseMap, overrideMap)
				continue
			}
		}
		result[key] = value
	}
	return result
}

func asMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func readYAMLMap(path string, required bool) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("配置文件不存在: %s", path)
	}
	var payload map[string]any
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("配置文件必须是合法 YAML: %s", path)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	return payload, nil
}

func applyYAMLOverride(endpoints *EndpointConfig, raw map[string]any) {
	if v := textOf(raw["service_tree"]); v != "" {
		endpoints.ServiceTree = v
	}
	if v := textOf(raw["service_detail"]); v != "" {
		endpoints.ServiceDetail = v
	}
	if v := textOf(raw["templates"]); v != "" {
		endpoints.Templates = v
	}
	if v := textOf(raw["tenant_information"]); v != "" {
		endpoints.TenantInformation = v
	}
	if v := textOf(raw["switch_tenant"]); v != "" {
		endpoints.SwitchTenant = v
	}
	if v := textOf(raw["switch_fiscal_year"]); v != "" {
		endpoints.SwitchFiscalYear = v
	}
	if v := textOf(raw["org_tree"]); v != "" {
		endpoints.OrgTree = v
	}
	if v := textOf(raw["update_default_org"]); v != "" {
		endpoints.UpdateDefaultOrg = v
	}
	if v := textOf(raw["user_context"]); v != "" {
		endpoints.UserContext = v
	}
}

func textOf(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
