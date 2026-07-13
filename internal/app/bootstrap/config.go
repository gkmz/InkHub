package bootstrap

// Config 保存启动时已合并的非敏感配置。
type Config struct {
	Host      string
	Port      int
	DataDir   string
	Workspace string
}

// Layer 表示一个配置来源及其显式设置字段。
type Layer struct {
	Name   string
	Values Config
	Set    map[string]bool
}

// MergedConfig 保存最终配置及每个字段的来源。
type MergedConfig struct {
	Config  Config
	Origins map[string]string
}

// Fields 创建显式配置字段集合。
func Fields(names ...string) map[string]bool {
	result := make(map[string]bool, len(names))
	for _, name := range names {
		result[name] = true
	}
	return result
}

// MergeConfig 按传入的低到高优先级合并配置。
func MergeConfig(layers ...Layer) MergedConfig {
	result := MergedConfig{Origins: make(map[string]string)}
	for index, layer := range layers {
		all := index == 0 && len(layer.Set) == 0
		if all || layer.Set["host"] {
			result.Config.Host = layer.Values.Host
			result.Origins["host"] = layer.Name
		}
		if all || layer.Set["port"] {
			result.Config.Port = layer.Values.Port
			result.Origins["port"] = layer.Name
		}
		if all || layer.Set["data_dir"] {
			result.Config.DataDir = layer.Values.DataDir
			result.Origins["data_dir"] = layer.Name
		}
		if all || layer.Set["workspace"] {
			result.Config.Workspace = layer.Values.Workspace
			result.Origins["workspace"] = layer.Name
		}
	}
	return result
}
