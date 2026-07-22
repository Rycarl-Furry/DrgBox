package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newCategoryTestApp(t *testing.T, categories string, tools []Tool) *App {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "categories.json"), []byte(categories), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(config{Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "tools.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	return &App{root: root}
}

func TestCategoryTreeMigratesLegacyAndMaintainsSubcategories(t *testing.T) {
	app := newCategoryTestApp(t, `{"categories":["框架漏洞利用工具","其他工具"]}`, []Tool{{ID: "1", Category: "框架漏洞利用工具/Spring"}})
	tree, err := app.GetCategoryTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 || tree[0].Name != "框架漏洞利用工具" || len(tree[0].Children) != 1 || tree[0].Children[0] != "Spring" {
		t.Fatalf("unexpected migrated tree: %#v", tree)
	}
	if err := app.AddSubcategory("框架漏洞利用工具", "ThinkPHP"); err != nil {
		t.Fatal(err)
	}
	if err := app.RenameSubcategory("框架漏洞利用工具", "Spring", "Spring Boot"); err != nil {
		t.Fatal(err)
	}
	tools, err := app.GetTools()
	if err != nil || tools[0].Category != "框架漏洞利用工具/Spring Boot" {
		t.Fatalf("tool path was not renamed: %#v, %v", tools, err)
	}
	if err := app.DeleteSubcategory("框架漏洞利用工具", "Spring Boot"); err != nil {
		t.Fatal(err)
	}
	tools, err = app.GetTools()
	if err != nil || tools[0].Category != "框架漏洞利用工具" {
		t.Fatalf("tool was not moved to parent: %#v, %v", tools, err)
	}
}

func TestRenameParentUpdatesChildToolPath(t *testing.T) {
	app := newCategoryTestApp(t, `{"categories":[{"name":"框架漏洞利用工具","children":["Spring"]}]}`, []Tool{{ID: "1", Category: "框架漏洞利用工具/Spring"}})
	if err := app.RenameCategory("框架漏洞利用工具", "框架漏洞"); err != nil {
		t.Fatal(err)
	}
	tools, err := app.GetTools()
	if err != nil || tools[0].Category != "框架漏洞/Spring" {
		t.Fatalf("parent rename did not update tool path: %#v, %v", tools, err)
	}
}

func TestMoveToolToCategory(t *testing.T) {
	app := newCategoryTestApp(t, `{"categories":[{"name":"信息收集","children":["域名收集"]},{"name":"其他工具"}]}`, []Tool{{ID: "1", Category: "其他工具"}})
	if err := app.MoveToolToCategory("1", "信息收集/域名收集"); err != nil {
		t.Fatal(err)
	}
	tools, err := app.GetTools()
	if err != nil || tools[0].Category != "信息收集/域名收集" {
		t.Fatalf("tool category was not updated: %#v, %v", tools, err)
	}
	if err := app.MoveToolToCategory("1", "不存在的分类"); err == nil {
		t.Fatal("expected invalid category error")
	}
}
