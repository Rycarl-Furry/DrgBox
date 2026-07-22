package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type Tool struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Path        string `json:"path"`
	Icon        string `json:"icon,omitempty"`
	Args        string `json:"args"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Tags        string `json:"tags"`
	Source      string `json:"source"`
	Weight      int    `json:"weight"`
	Favorite    bool   `json:"favorite"`
	LastRun     int64  `json:"lastRun"`
}
type config struct {
	Tools []Tool `json:"tools"`
}
type CategoryNode struct {
	Name     string   `json:"name"`
	Children []string `json:"children,omitempty"`
}
type categoryConfig struct {
	Categories json.RawMessage `json:"categories"`
}
type SyncSummary struct {
	Added int `json:"added"`
	Total int `json:"total"`
}

// RuntimeSettings 保存用户自定义的解释器/JRE 入口。字段为空时使用内置运行环境，
// 内置环境不存在时再尝试系统 PATH。
type RuntimeSettings struct {
	PythonPath string `json:"pythonPath"`
	Java8Path  string `json:"java8Path"`
	Java11Path string `json:"java11Path"`
}

type appSettings struct {
	QuickHotkey string `json:"quickHotkey,omitempty"`
	PythonPath  string `json:"pythonPath,omitempty"`
	Java8Path   string `json:"java8Path,omitempty"`
	Java11Path  string `json:"java11Path,omitempty"`
}

var defaultCategories = []string{"全部工具", "最近启动", "我的收藏", "WebShell管理工具", "信息收集工具", "抓包与代理工具", "漏洞扫描与利用工具", "框架漏洞利用工具", "爆破工具", "免杀工具", "后渗透工具", "其他工具", "网页工具"}

type App struct {
	ctx        context.Context
	root       string
	settingsMu sync.Mutex
}

func NewApp() *App { return &App{} }
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.root = locateRoot()
	go registerToggleHotkey(ctx, a.GetQuickHotkey())
	go startTray(ctx)
}

func (a *App) settingsPath() string { return filepath.Join(a.root, "data", "settings.json") }
func (a *App) readSettings() appSettings {
	var settings appSettings
	b, err := os.ReadFile(a.settingsPath())
	if err == nil {
		_ = json.Unmarshal(b, &settings)
	}
	return settings
}
func (a *App) writeSettings(settings appSettings) error {
	if err := os.MkdirAll(filepath.Dir(a.settingsPath()), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.settingsPath(), b, 0644)
}
func (a *App) GetQuickHotkey() string {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	v := a.readSettings()
	if v.QuickHotkey == "" {
		return "Ctrl+Shift+Space"
	}
	return v.QuickHotkey
}
func (a *App) SetQuickHotkey(combo string) error {
	if !updateQuickHotkey(combo) {
		return fmt.Errorf("快捷键格式无效")
	}
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	settings := a.readSettings()
	settings.QuickHotkey = combo
	return a.writeSettings(settings)
}

func (a *App) GetRuntimeSettings() RuntimeSettings {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	settings := a.readSettings()
	return RuntimeSettings{PythonPath: settings.PythonPath, Java8Path: settings.Java8Path, Java11Path: settings.Java11Path}
}

// SaveRuntimeSettings 校验后保存为精确的可执行文件路径。输入也可以是 Python/JDK/JRE 根目录或 bin 目录。
func (a *App) SaveRuntimeSettings(value RuntimeSettings) error {
	paths := []*string{&value.PythonPath, &value.Java8Path, &value.Java11Path}
	kinds := []string{"python", "java8", "java11"}
	labels := []string{"Python", "Java 8", "Java 11"}
	for i, path := range paths {
		*path = strings.Trim(strings.TrimSpace(*path), `"`)
		if *path == "" {
			continue
		}
		resolved, err := resolveConfiguredRuntime(kinds[i], *path)
		if err != nil {
			return fmt.Errorf("%s 路径无效：%w", labels[i], err)
		}
		*path = resolved
	}

	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	settings := a.readSettings()
	settings.PythonPath = value.PythonPath
	settings.Java8Path = value.Java8Path
	settings.Java11Path = value.Java11Path
	return a.writeSettings(settings)
}

// ChooseRuntimeExecutable 选择解释器或 java/javaw 可执行文件；也可以在界面中直接填写安装目录。
func (a *App) ChooseRuntimeExecutable(kind string) (string, error) {
	title := "选择运行环境"
	if strings.EqualFold(kind, "python") {
		title = "选择 Python 解释器"
	} else if strings.EqualFold(kind, "java8") {
		title = "选择 Java 8（javaw.exe 或 java.exe）"
	} else if strings.EqualFold(kind, "java11") {
		title = "选择 Java 11（javaw.exe 或 java.exe）"
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   title,
		Filters: []runtime.FileFilter{{DisplayName: "可执行文件", Pattern: "*.exe;*.cmd;*.bat"}},
	})
}
func (a *App) ExitQuickLauncher() {
	runtime.WindowSetSize(a.ctx, 1440, 900)
	runtime.WindowCenter(a.ctx)
	runtime.EventsEmit(a.ctx, "drgbox:quick-close")
}

func locateRoot() string {
	starts := []string{}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		for p := start; ; p = filepath.Dir(p) {
			if _, err := os.Stat(filepath.Join(p, "data", "tools.json")); err == nil {
				return p
			}
			next := filepath.Dir(p)
			if next == p {
				break
			}
		}
	}
	return `D:\Car1N0tCat`
}
func (a *App) configPath() string   { return filepath.Join(a.root, "data", "tools.json") }
func (a *App) categoryPath() string { return filepath.Join(a.root, "data", "categories.json") }
func (a *App) runtimePath() string  { return filepath.Join(a.root, "runtime") }

func runtimeCandidates(kind, base string) []string {
	switch strings.ToLower(kind) {
	case "python":
		return []string{
			base,
			filepath.Join(base, "python.exe"),
			filepath.Join(base, "python3.exe"),
			filepath.Join(base, "bin", "python.exe"),
			filepath.Join(base, "bin", "python3.exe"),
		}
	default:
		return []string{
			base,
			filepath.Join(base, "javaw.exe"),
			filepath.Join(base, "java.exe"),
			filepath.Join(base, "bin", "javaw.exe"),
			filepath.Join(base, "bin", "java.exe"),
		}
	}
}

func resolveConfiguredRuntime(kind, value string) (string, error) {
	value = os.ExpandEnv(strings.Trim(strings.TrimSpace(value), `"`))
	if value == "" {
		return "", fmt.Errorf("路径为空")
	}
	if !strings.ContainsAny(value, `\/:`) {
		if found, err := exec.LookPath(value); err == nil {
			return found, nil
		}
	}
	for _, candidate := range runtimeCandidates(kind, filepath.Clean(value)) {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return filepath.Clean(candidate), nil
		}
	}
	return "", fmt.Errorf("未找到可执行文件（可填写 exe、安装根目录或 bin 目录）")
}

func (a *App) runtimeExecutable(kind string) (string, error) {
	a.settingsMu.Lock()
	settings := a.readSettings()
	a.settingsMu.Unlock()

	configured := settings.PythonPath
	if strings.EqualFold(kind, "java8") {
		configured = settings.Java8Path
	} else if strings.EqualFold(kind, "java11") {
		configured = settings.Java11Path
	}
	if strings.TrimSpace(configured) != "" {
		return resolveConfiguredRuntime(kind, configured)
	}

	var defaults []string
	switch strings.ToLower(kind) {
	case "python":
		defaults = []string{filepath.Join(a.runtimePath(), "python3", "python.exe"), "python.exe", "python3.exe"}
	case "java11":
		defaults = []string{filepath.Join(a.runtimePath(), "Java_path", "Java_11_win", "bin", "javaw.exe"), filepath.Join(a.runtimePath(), "Java_path", "Java_11_win", "bin", "java.exe"), "javaw.exe", "java.exe"}
	default:
		defaults = []string{filepath.Join(a.runtimePath(), "Java_path", "Java_8_win", "bin", "javaw.exe"), filepath.Join(a.runtimePath(), "Java_path", "Java_8_win", "bin", "java.exe"), "javaw.exe", "java.exe"}
	}
	for _, candidate := range defaults {
		if strings.ContainsAny(candidate, `\/:`) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
			continue
		}
		if found, err := exec.LookPath(candidate); err == nil {
			return found, nil
		}
	}
	return "", fmt.Errorf("未找到 %s 运行环境，请在外观设置的“运行环境”中配置", strings.ToUpper(kind))
}

func (a *App) toolByID(id string) (Tool, error) {
	items, err := a.GetTools()
	if err != nil {
		return Tool{}, err
	}
	for _, tool := range items {
		if tool.ID == id {
			return tool, nil
		}
	}
	return Tool{}, fmt.Errorf("未找到工具：%s", id)
}

func (a *App) GetTools() ([]Tool, error) {
	b, err := os.ReadFile(a.configPath())
	if err != nil {
		return nil, err
	}
	var c config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return c.Tools, nil
}
func (a *App) saveTools(items []Tool) error {
	b, err := json.MarshalIndent(config{Tools: items}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.configPath(), b, 0644)
}
func (a *App) ToggleFavorite(id string) error {
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == id {
			items[i].Favorite = !items[i].Favorite
			break
		}
	}
	return a.saveTools(items)
}

func cleanCategoryName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("分类名称不能为空")
	}
	if strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("分类名称不能包含 / 或 \\")
	}
	return name, nil
}

func splitCategoryPath(value string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(value), ""
}

// loadCategoryTree 同时兼容旧版字符串数组和新版两级分类对象数组。
func (a *App) loadCategoryTree() ([]CategoryNode, error) {
	nodes := []CategoryNode{}
	loaded := false
	if b, err := os.ReadFile(a.categoryPath()); err == nil {
		var cfg categoryConfig
		if json.Unmarshal(b, &cfg) == nil && cfg.Categories != nil {
			var entries []json.RawMessage
			if json.Unmarshal(cfg.Categories, &entries) == nil {
				loaded = true
				for _, entry := range entries {
					var oldName string
					if json.Unmarshal(entry, &oldName) == nil {
						nodes = append(nodes, CategoryNode{Name: oldName})
						continue
					}
					var node CategoryNode
					if json.Unmarshal(entry, &node) == nil {
						nodes = append(nodes, node)
					}
				}
			}
		}
	}
	if !loaded {
		for _, name := range defaultCategories[3:] {
			nodes = append(nodes, CategoryNode{Name: name})
		}
	}

	// 规范化并去重，工具中已经使用的二级路径也会补回分类树。
	clean := []CategoryNode{}
	index := map[string]int{}
	add := func(parent, child string) {
		parent, child = strings.TrimSpace(parent), strings.TrimSpace(child)
		if parent == "" {
			return
		}
		i, ok := index[parent]
		if !ok {
			i = len(clean)
			index[parent] = i
			clean = append(clean, CategoryNode{Name: parent})
		}
		if child != "" {
			for _, existing := range clean[i].Children {
				if existing == child {
					return
				}
			}
			clean[i].Children = append(clean[i].Children, child)
		}
	}
	for _, node := range nodes {
		for _, child := range append([]string{""}, node.Children...) {
			add(node.Name, child)
		}
	}
	if tools, err := a.GetTools(); err == nil {
		for _, tool := range tools {
			parent, child := splitCategoryPath(tool.Category)
			add(parent, child)
		}
	}
	return clean, nil
}

func (a *App) GetCategoryTree() ([]CategoryNode, error) { return a.loadCategoryTree() }

// GetCategories 保留给旧前端/脚本使用，返回系统视图和可直接用于工具归类的路径。
func (a *App) GetCategories() ([]string, error) {
	tree, err := a.loadCategoryTree()
	if err != nil {
		return nil, err
	}
	out := append([]string{}, defaultCategories[:3]...)
	for _, node := range tree {
		out = append(out, node.Name)
		for _, child := range node.Children {
			out = append(out, node.Name+"/"+child)
		}
	}
	return out, nil
}

func (a *App) saveCategoryTree(nodes []CategoryNode) error {
	seen := map[string]bool{}
	stored := make([]CategoryNode, 0, len(nodes))
	for _, node := range nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		childSeen := map[string]bool{}
		children := []string{}
		for _, child := range node.Children {
			child = strings.TrimSpace(child)
			if child != "" && !childSeen[child] {
				childSeen[child] = true
				children = append(children, child)
			}
		}
		stored = append(stored, CategoryNode{Name: name, Children: children})
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(categoryConfig{Categories: raw}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.categoryPath(), b, 0644)
}

func (a *App) AddCategory(name string) error {
	var err error
	if name, err = cleanCategoryName(name); err != nil {
		return err
	}
	tree, _ := a.loadCategoryTree()
	for _, node := range tree {
		if node.Name == name {
			return nil
		}
	}
	return a.saveCategoryTree(append(tree, CategoryNode{Name: name}))
}

func (a *App) AddSubcategory(parent, name string) error {
	var err error
	parent = strings.TrimSpace(parent)
	if name, err = cleanCategoryName(name); err != nil {
		return err
	}
	tree, err := a.loadCategoryTree()
	if err != nil {
		return err
	}
	for i := range tree {
		if tree[i].Name != parent {
			continue
		}
		for _, child := range tree[i].Children {
			if child == name {
				return fmt.Errorf("已存在同名二级分类")
			}
		}
		tree[i].Children = append(tree[i].Children, name)
		return a.saveCategoryTree(tree)
	}
	return fmt.Errorf("未找到一级分类")
}

// RenameCategory 会同步重命名已归类工具，工具文件本身不会被移动或修改。
func (a *App) RenameCategory(oldName, newName string) error {
	oldName = strings.TrimSpace(oldName)
	var err error
	if newName, err = cleanCategoryName(newName); err != nil {
		return err
	}
	for _, view := range defaultCategories[:3] {
		if oldName == view {
			return fmt.Errorf("系统视图不能修改")
		}
	}
	if oldName == newName {
		return nil
	}
	tree, err := a.loadCategoryTree()
	if err != nil {
		return err
	}
	found := false
	for _, node := range tree {
		if node.Name == oldName {
			found = true
		}
		if node.Name == newName {
			return fmt.Errorf("已存在同名分类")
		}
	}
	if !found {
		return fmt.Errorf("未找到分类")
	}
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	for i := range items {
		parent, child := splitCategoryPath(items[i].Category)
		if parent == oldName {
			items[i].Category = newName
			if child != "" {
				items[i].Category += "/" + child
			}
		}
	}
	if err := a.saveTools(items); err != nil {
		return err
	}
	for i := range tree {
		if tree[i].Name == oldName {
			tree[i].Name = newName
		}
	}
	return a.saveCategoryTree(tree)
}

func (a *App) RenameSubcategory(parent, oldName, newName string) error {
	parent, oldName = strings.TrimSpace(parent), strings.TrimSpace(oldName)
	var err error
	if newName, err = cleanCategoryName(newName); err != nil {
		return err
	}
	tree, err := a.loadCategoryTree()
	if err != nil {
		return err
	}
	found := false
	for i := range tree {
		if tree[i].Name != parent {
			continue
		}
		for _, child := range tree[i].Children {
			if child == newName && child != oldName {
				return fmt.Errorf("已存在同名二级分类")
			}
		}
		for j := range tree[i].Children {
			if tree[i].Children[j] == oldName {
				tree[i].Children[j] = newName
				found = true
			}
		}
	}
	if !found {
		return fmt.Errorf("未找到二级分类")
	}
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	oldPath, newPath := parent+"/"+oldName, parent+"/"+newName
	for i := range items {
		if items[i].Category == oldPath {
			items[i].Category = newPath
		}
	}
	if err := a.saveTools(items); err != nil {
		return err
	}
	return a.saveCategoryTree(tree)
}

func (a *App) DeleteCategory(name string) error {
	name = strings.TrimSpace(name)
	for _, view := range defaultCategories[:3] {
		if name == view {
			return fmt.Errorf("系统视图不能删除")
		}
	}
	if name == "" {
		return fmt.Errorf("请选择要删除的分类")
	}
	tree, err := a.loadCategoryTree()
	if err != nil {
		return err
	}
	found := false
	fallback := ""
	for _, node := range tree {
		if node.Name == name {
			found = true
			continue
		}
		if fallback == "" {
			fallback = node.Name
		}
	}
	if !found {
		return fmt.Errorf("未找到分类")
	}
	if fallback == "" {
		fallback = "未分类工具"
		tree = append(tree, CategoryNode{Name: fallback})
	}
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	for i := range items {
		parent, _ := splitCategoryPath(items[i].Category)
		if parent == name {
			items[i].Category = fallback
		}
	}
	if err := a.saveTools(items); err != nil {
		return err
	}
	next := []CategoryNode{}
	for _, node := range tree {
		if node.Name != name {
			next = append(next, node)
		}
	}
	return a.saveCategoryTree(next)
}

func (a *App) DeleteSubcategory(parent, name string) error {
	parent, name = strings.TrimSpace(parent), strings.TrimSpace(name)
	tree, err := a.loadCategoryTree()
	if err != nil {
		return err
	}
	found := false
	for i := range tree {
		if tree[i].Name != parent {
			continue
		}
		next := []string{}
		for _, child := range tree[i].Children {
			if child == name {
				found = true
			} else {
				next = append(next, child)
			}
		}
		tree[i].Children = next
	}
	if !found {
		return fmt.Errorf("未找到二级分类")
	}
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	path := parent + "/" + name
	for i := range items {
		if items[i].Category == path {
			items[i].Category = parent
		}
	}
	if err := a.saveTools(items); err != nil {
		return err
	}
	return a.saveCategoryTree(tree)
}

// MoveCategory 将普通分类拖动到另一分类之前，三个快捷视图保持固定。
func (a *App) MoveCategory(name, before string) error {
	if name == "" || before == "" || name == before {
		return nil
	}
	for _, fixed := range defaultCategories[:3] {
		if name == fixed || before == fixed {
			return fmt.Errorf("快捷视图不能调整位置")
		}
	}
	tree, err := a.loadCategoryTree()
	if err != nil {
		return err
	}
	next := make([]CategoryNode, 0, len(tree))
	found := false
	for _, node := range tree {
		if node.Name == name {
			found = true
			continue
		}
		next = append(next, node)
	}
	if !found {
		return fmt.Errorf("未找到分类")
	}
	insert := -1
	for i, node := range next {
		if node.Name == before {
			insert = i
			break
		}
	}
	if insert < 0 {
		return fmt.Errorf("未找到目标分类")
	}
	next = append(next, CategoryNode{})
	copy(next[insert+1:], next[insert:])
	for _, node := range tree {
		if node.Name == name {
			next[insert] = node
			break
		}
	}
	return a.saveCategoryTree(next)
}
func (a *App) AddTool(t Tool) error {
	t.Name, t.Path, t.Category = strings.TrimSpace(t.Name), strings.TrimSpace(t.Path), strings.TrimSpace(t.Category)
	if t.Name == "" || t.Path == "" {
		return fmt.Errorf("工具名称和启动路径不能为空")
	}
	if _, err := os.Stat(t.Path); err != nil {
		return fmt.Errorf("启动路径无效：%w", err)
	}
	if t.Category == "" {
		t.Category = "其他工具"
	}
	if t.Type == "" {
		t.Type = detectType(t.Path)
	}
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	for _, x := range items {
		if strings.EqualFold(filepath.Clean(x.Path), filepath.Clean(t.Path)) {
			return fmt.Errorf("该启动路径已存在：%s", x.Name)
		}
	}
	t.ID = stableID(t.Path + time.Now().String())
	t.Source = "manual"
	items = append(items, t)
	return a.saveTools(items)
}
func (a *App) DeleteTool(id string) error {
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	next := make([]Tool, 0, len(items))
	found := false
	for _, t := range items {
		if t.ID == id {
			found = true
			continue
		}
		next = append(next, t)
	}
	if !found {
		return fmt.Errorf("未找到工具")
	}
	return a.saveTools(next)
}

// UpdateTool 更新 JSON 中已有工具的启动方式、分类和描述，不触碰磁盘文件。
func (a *App) UpdateTool(t Tool) error {
	t.ID, t.Name, t.Path, t.Category = strings.TrimSpace(t.ID), strings.TrimSpace(t.Name), strings.TrimSpace(t.Path), strings.TrimSpace(t.Category)
	if t.ID == "" || t.Name == "" || t.Path == "" {
		return fmt.Errorf("工具名称和启动路径不能为空")
	}
	if _, err := os.Stat(t.Path); err != nil {
		return fmt.Errorf("启动路径无效：%w", err)
	}
	if t.Category == "" {
		t.Category = "其他工具"
	}
	if t.Type == "" {
		t.Type = detectType(t.Path)
	}
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == t.ID {
			t.Favorite = items[i].Favorite
			t.LastRun = items[i].LastRun
			t.Source = items[i].Source
			if t.Source == "" {
				t.Source = "manual"
			}
			items[i] = t
			return a.saveTools(items)
		}
	}
	return fmt.Errorf("未找到工具")
}

// MoveToolToCategory 修改工具归属。拖到一级分类时保存为一级路径，
// 拖到二级分类时保存为“一级/二级”，不会移动或修改磁盘中的工具文件。
func (a *App) MoveToolToCategory(id, target string) error {
	id, target = strings.TrimSpace(id), strings.TrimSpace(target)
	if id == "" || target == "" {
		return fmt.Errorf("工具和目标分类不能为空")
	}
	for _, view := range defaultCategories[:3] {
		if target == view {
			return fmt.Errorf("不能把工具移动到系统视图")
		}
	}
	parent, child := splitCategoryPath(target)
	tree, err := a.loadCategoryTree()
	if err != nil {
		return err
	}
	valid := false
	for _, node := range tree {
		if node.Name != parent {
			continue
		}
		if child == "" {
			valid = true
			break
		}
		for _, name := range node.Children {
			if name == child {
				valid = true
				break
			}
		}
		break
	}
	if !valid {
		return fmt.Errorf("目标分类不存在：%s", target)
	}
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].ID == id {
			if items[i].Category == target {
				return nil
			}
			items[i].Category = target
			return a.saveTools(items)
		}
	}
	return fmt.Errorf("未找到工具")
}

// MoveTool 将一个工具拖到目标工具之前，仅调整 tools.json 的展示顺序。
func (a *App) MoveTool(id, before string) error {
	if id == "" || before == "" || id == before {
		return nil
	}
	items, err := a.GetTools()
	if err != nil {
		return err
	}
	from := -1
	for i := range items {
		if items[i].ID == id {
			from = i
			break
		}
	}
	if from < 0 {
		return fmt.Errorf("未找到工具")
	}
	moving := items[from]
	items = append(items[:from], items[from+1:]...)
	to := -1
	for i := range items {
		if items[i].ID == before {
			to = i
			break
		}
	}
	if to < 0 {
		return fmt.Errorf("未找到目标工具")
	}
	items = append(items, Tool{})
	copy(items[to+1:], items[to:])
	items[to] = moving
	return a.saveTools(items)
}

// CleanInvalidTools 只清理 tools.json 内被误当作工具的目录、数据库、压缩包和缓存项；不删除磁盘原文件。
func (a *App) CleanInvalidTools() (int, error) {
	items, err := a.GetTools()
	if err != nil {
		return 0, err
	}
	next := make([]Tool, 0, len(items))
	removed := 0
	for _, t := range items {
		typ := strings.TrimSpace(t.Type)
		ext := strings.ToLower(filepath.Ext(t.Path))
		invalid := typ == "目录" || ext == ".db" || ext == ".mdb" || ext == ".mv" || ext == ".zip" || ext == ".cfg"
		if invalid {
			removed++
			continue
		}
		next = append(next, t)
	}
	return removed, a.saveTools(next)
}

// ChooseWallpaper 使用原生文件选择器导入图片或 MP4/WebM，并复制到本地 data 目录。
// 文件以流式复制保存，不经 base64/localStorage，因此没有大小限制。
func (a *App) ChooseWallpaper() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择壁纸（图片或视频）", Filters: []runtime.FileFilter{{DisplayName: "壁纸文件", Pattern: "*.mp4;*.webm;*.mov;*.mkv;*.png;*.jpg;*.jpeg;*.webp"}}})
	if err != nil || path == "" {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(path))
	dstDir := filepath.Join(a.root, "data", "wallpaper")
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return "", err
	}
	dst := filepath.Join(dstDir, "current"+ext)
	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	setting, _ := json.Marshal(map[string]string{"path": dst})
	if err := os.WriteFile(filepath.Join(dstDir, "settings.json"), setting, 0644); err != nil {
		return "", err
	}
	return "/wallpaper?ext=" + ext, nil
}
func (a *App) GetWallpaper() string {
	b, err := os.ReadFile(filepath.Join(a.root, "data", "wallpaper", "settings.json"))
	if err != nil {
		return ""
	}
	var v struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(b, &v) != nil || v.Path == "" {
		return ""
	}
	return "/wallpaper?ext=" + strings.ToLower(filepath.Ext(v.Path))
}
func (a *App) ClearWallpaper() error {
	return os.Remove(filepath.Join(a.root, "data", "wallpaper", "settings.json"))
}
func (a *App) ServeWallpaper(w http.ResponseWriter, r *http.Request) {
	b, err := os.ReadFile(filepath.Join(a.root, "data", "wallpaper", "settings.json"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var v struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(b, &v) != nil || v.Path == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, v.Path)
}

// ChooseAvatar 导入顶栏头像，保存到 data/avatar 并经本地资源路由展示。
func (a *App) ChooseAvatar() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "选择头像", Filters: []runtime.FileFilter{{DisplayName: "图片文件", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.gif"}}})
	if err != nil || path == "" {
		return "", err
	}
	dstDir := filepath.Join(a.root, "data", "avatar")
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(path))
	dst := filepath.Join(dstDir, "current"+ext)
	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	setting, _ := json.Marshal(map[string]string{"path": dst})
	if err := os.WriteFile(filepath.Join(dstDir, "settings.json"), setting, 0644); err != nil {
		return "", err
	}
	return "/avatar?ext=" + ext, nil
}

func (a *App) GetAvatar() string {
	b, err := os.ReadFile(filepath.Join(a.root, "data", "avatar", "settings.json"))
	if err != nil {
		return ""
	}
	var v struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(b, &v) != nil || v.Path == "" {
		return ""
	}
	return "/avatar?ext=" + strings.ToLower(filepath.Ext(v.Path))
}

func (a *App) ServeAvatar(w http.ResponseWriter, r *http.Request) {
	b, err := os.ReadFile(filepath.Join(a.root, "data", "avatar", "settings.json"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var v struct {
		Path string `json:"path"`
	}
	if json.Unmarshal(b, &v) != nil || v.Path == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, v.Path)
}

// ChooseToolIcon 选择工具的自定义图标。留空时前端使用 Windows 关联图标。
func (a *App) ChooseToolIcon() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择工具图标",
		Filters: []runtime.FileFilter{{
			DisplayName: "图标或图片文件",
			Pattern:     "*.ico;*.png;*.jpg;*.jpeg;*.webp;*.gif",
		}},
	})
}

func (a *App) OpenToolDirectory(id string) error {
	tool, err := a.toolByID(id)
	if err != nil {
		return err
	}
	info, err := os.Stat(tool.Path)
	if err != nil {
		return fmt.Errorf("工具路径不存在：%w", err)
	}
	if info.IsDir() {
		return exec.Command("explorer.exe", filepath.Clean(tool.Path)).Start()
	}
	return exec.Command("explorer.exe", "/select,", filepath.Clean(tool.Path)).Start()
}

func (a *App) ServeToolIcon(w http.ResponseWriter, r *http.Request) {
	tool, err := a.toolByID(strings.TrimSpace(r.URL.Query().Get("id")))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if tool.Icon != "" {
		if info, statErr := os.Stat(tool.Icon); statErr == nil && !info.IsDir() {
			w.Header().Set("Cache-Control", "public, max-age=300")
			http.ServeFile(w, r, tool.Icon)
			return
		}
	}
	cacheDir := filepath.Join(a.root, "data", "icons")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		http.NotFound(w, r)
		return
	}
	cachePath := filepath.Join(cacheDir, tool.ID+"-"+stableID(tool.Path)+".png")
	if _, err := os.Stat(cachePath); err != nil {
		if err := extractAssociatedIcon(tool.Path, cachePath); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, cachePath)
}

// SyncToolDirectory 将每个一级分类目录下的工具入口写入 JSON；已有路径不会重复添加。
func (a *App) SyncToolDirectory() (SyncSummary, error) {
	root := `D:\SRC渗透工具分类`
	if _, err := os.Stat(root); err != nil {
		return SyncSummary{}, err
	}
	items, err := a.GetTools()
	if err != nil {
		return SyncSummary{}, err
	}
	known := map[string]bool{}
	for _, t := range items {
		known[strings.ToLower(filepath.Clean(t.Path))] = true
	}
	groups, err := os.ReadDir(root)
	if err != nil {
		return SyncSummary{}, err
	}
	added := 0
	for _, group := range groups {
		if !group.IsDir() {
			continue
		}
		cat := classifyFolder(group.Name())
		base := filepath.Join(root, group.Name())
		entries, _ := os.ReadDir(base)
		for _, entry := range entries {
			path := filepath.Join(base, entry.Name())
			launch := ""
			if entry.IsDir() {
				launch = findLaunch(path, entry.Name())
			} else if isLaunchFile(path) {
				launch = path
			}
			if launch == "" {
				continue
			}
			key := strings.ToLower(filepath.Clean(launch))
			if known[key] {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if entry.IsDir() {
				name = cleanToolName(entry.Name())
			}
			items = append(items, Tool{ID: stableID(launch), Name: name, Type: detectType(launch), Path: launch, Category: cat, Description: shortDesc(name), Source: "sync"})
			known[key] = true
			added++
		}
	}
	if added > 0 {
		if err := a.saveTools(items); err != nil {
			return SyncSummary{}, err
		}
	}
	return SyncSummary{Added: added, Total: len(items)}, nil
}
func findLaunch(dir, folder string) string {
	best := ""
	bestScore := -999
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		depth := strings.Count(rel, string(os.PathSeparator))
		if d.IsDir() {
			n := strings.ToLower(d.Name())
			if depth > 2 || strings.Contains(n, "node_modules") || strings.Contains(n, "resources") || strings.Contains(n, "plugins") || strings.Contains(n, "common") || strings.Contains(n, "lib") {
				return filepath.SkipDir
			}
			return nil
		}
		if depth > 2 || !isLaunchFile(path) {
			return nil
		}
		score := launchScore(path, folder, depth)
		if score > bestScore {
			bestScore, best = score, path
		}
		return nil
	})
	return best
}
func isLaunchFile(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".exe", ".bat", ".cmd", ".jar", ".py", ".ps1":
		return true
	}
	return false
}
func launchScore(path, folder string, depth int) int {
	n := strings.ToLower(filepath.Base(path))
	f := strings.ToLower(folder)
	s := 100 - depth*15
	if strings.Contains(n, "main") || strings.Contains(n, "start") || strings.Contains(n, "app") {
		s += 45
	}
	if strings.TrimSuffix(n, filepath.Ext(n)) == f {
		s += 55
	}
	switch filepath.Ext(n) {
	case ".exe":
		s += 40
	case ".bat", ".cmd":
		s += 30
	case ".jar":
		s += 25
	case ".py":
		s += 10
	}
	if strings.Contains(n, "test") || strings.Contains(n, "demo") || strings.Contains(n, "loader") {
		s -= 80
	}
	return s
}
func detectType(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".py":
		return "Python"
	case ".jar":
		return "JAVA8"
	case ".bat", ".cmd":
		return "批处理"
	case ".ps1":
		return "PowerShell"
	case ".exe":
		return "GUI应用"
	}
	return "命令行"
}
func stableID(s string) string {
	h := sha1.Sum([]byte(strings.ToLower(filepath.Clean(s))))
	return hex.EncodeToString(h[:])[:16]
}
func cleanToolName(s string) string {
	if i := strings.Index(s, " - "); i > 0 {
		return s[:i]
	}
	return s
}
func classifyFolder(s string) string {
	l := strings.ToLower(s)
	switch {
	case strings.Contains(l, "webshell"):
		return "WebShell管理工具"
	case strings.Contains(l, "资产") || strings.Contains(l, "子域") || strings.Contains(l, "目录") || strings.Contains(l, "指纹"):
		return "信息收集工具"
	case strings.Contains(l, "burp") || strings.Contains(l, "抓包") || strings.Contains(l, "代理"):
		return "抓包与代理工具"
	case strings.Contains(l, "框架") || strings.Contains(l, "oa") || strings.Contains(l, "cms") || strings.Contains(l, "中间件"):
		return "框架漏洞利用工具"
	case strings.Contains(l, "爆破") || strings.Contains(l, "弱口令") || strings.Contains(l, "字典"):
		return "爆破工具"
	case strings.Contains(l, "免杀"):
		return "免杀工具"
	case strings.Contains(l, "内网") || strings.Contains(l, "横向") || strings.Contains(l, "域") || strings.Contains(l, "凭据") || strings.Contains(l, "提权"):
		return "后渗透工具"
	case strings.Contains(l, "漏洞") || strings.Contains(l, "sql") || strings.Contains(l, "poc"):
		return "漏洞扫描与利用工具"
	case strings.Contains(l, "编码") || strings.Contains(l, "ctf"):
		return "网页工具"
	}
	return "其他工具"
}
func shortDesc(n string) string {
	if n == "" {
		return "本地工具"
	}
	return n + " 本地启动工具"
}

func (a *App) RunTool(id string) (string, error) {
	items, err := a.GetTools()
	if err != nil {
		return "", err
	}
	for i := range items {
		if items[i].ID == id {
			if err := a.launch(&items[i]); err != nil {
				return "", err
			}
			items[i].LastRun = time.Now().Unix()
			_ = a.saveTools(items)
			return items[i].Name, nil
		}
	}
	return "", fmt.Errorf("未找到工具：%s", id)
}

// consoleCommand 生成交给 cmd.exe /k 的单条命令。工作目录由 exec.Cmd.Dir 设置，
// 避免 start、cd 和多层引号在中文/空格路径下被二次解析。
func consoleCommand(executable, arguments string) string {
	command := `call "` + strings.ReplaceAll(filepath.Clean(executable), `"`, `""`) + `"`
	if arguments = strings.TrimSpace(arguments); arguments != "" {
		command += " " + arguments
	}
	return command
}

// commandPrompt 使用 Windows 原生命令行字符串，避免 os/exec 再把内部引号转义成 \"。
func commandPrompt(command string, keepOpen bool) *exec.Cmd {
	mode := "/c"
	if keepOpen {
		mode = "/k"
	}
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe /d " + mode + " " + command}
	return cmd
}

func (a *App) launch(t *Tool) error {
	info, err := os.Stat(t.Path)
	if err != nil {
		return fmt.Errorf("启动路径不存在：%w", err)
	}
	if info.IsDir() {
		// start 带空标题可正确处理中文、空格和末尾反斜杠，直接定位到用户配置的目录。
		return exec.Command("cmd.exe", "/c", "start", "", filepath.Clean(t.Path)).Start()
	}
	args := strings.Fields(t.Args)
	dir := filepath.Dir(t.Path)
	typ := strings.ToLower(t.Type)
	var cmd *exec.Cmd
	switch {
	case typ == "python":
		py, runtimeErr := a.runtimeExecutable("python")
		if runtimeErr != nil {
			return runtimeErr
		}
		pythonArgs := `"` + strings.ReplaceAll(filepath.Clean(t.Path), `"`, `""`) + `"`
		if strings.TrimSpace(t.Args) != "" {
			pythonArgs += " " + strings.TrimSpace(t.Args)
		}
		cmd = commandPrompt(consoleCommand(py, pythonArgs), true)
	case typ == "java8" || typ == "java11":
		java, runtimeErr := a.runtimeExecutable(typ)
		if runtimeErr != nil {
			return runtimeErr
		}
		cmd = exec.Command(java, append([]string{"-jar", t.Path}, args...)...)
	case typ == "批处理":
		// 使用 call 直接执行 .bat/.cmd，且通过 cmd.Dir 设置工作目录。
		// 不能把 cd、start 与脚本路径拼成一条命令，否则中文路径和引号会被 CMD 二次解析而报“文件名、目录名或卷标语法不正确”。
		command := fmt.Sprintf(`call "%s"`, t.Path)
		if strings.TrimSpace(t.Args) != "" {
			command += " " + t.Args
		}
		cmd = commandPrompt(command, true)
	case typ == "命令行":
		cmd = commandPrompt(consoleCommand(t.Path, t.Args), true)
	case typ == "powershell":
		psArgs := []string{"-NoExit", "-ExecutionPolicy", "Bypass", "-File", t.Path}
		psArgs = append(psArgs, args...)
		cmd = exec.Command("powershell.exe", psArgs...)
	default:
		// GUI 程序不能使用 HideWindow；蚁剑等 Electron 应用会连主窗口一起被隐藏。
		cmd = exec.Command(t.Path, args...)
	}
	cmd.Dir = dir
	return cmd.Start()
}
