package settings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLegacyRulesGainPersistentIdentitiesWithoutRewritingOnLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	source := "[[programs.editors]]\next = ['txt', 'md']\npath = 'editor.exe'\n[[programs.editors]]\next = ['*']\npath = 'fallback.exe'\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := LoadAndRepair(dir, CatalogueDefault)
	if err != nil {
		t.Fatal("missing IDs triggered recovery", err)
	}
	rows := values.Collection(EditorCollection)
	first, second := rows[0]["id"].(string), rows[1]["id"].(string)
	if len(first) != 12 || len(second) != 12 || first == second || !ValidIdentity(first) || !ValidIdentity(second) {
		t.Fatal("generated IDs are not unique compact identities", rows)
	}
	data, _ := os.ReadFile(path)
	if string(data) != source {
		t.Fatal("load rewrote legacy settings")
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatal("missing IDs created a recovery backup", err)
	}
	if err := Save(dir, values); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAndRepair(dir, CatalogueDefault)
	if err != nil || !reflect.DeepEqual(loaded.Collection(EditorCollection), rows) {
		t.Fatal("IDs or rule data changed after saving", err)
	}
}

func TestCollectionIdentityRecoveryKeepsRulesAndFirstOwner(t *testing.T) {
	for _, invalid := range []string{"'123'", "'bad.id'", "123", "''"} {
		t.Run(invalid, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.toml")
			source := "appearance.autocolor.salt = 42\n[[programs.editors]]\nid = '123'\next = ['txt']\npath = 'first.exe'\n[[programs.editors]]\nid = " + invalid + "\next = ['md']\npath = 'second.exe'\n"
			if err := os.WriteFile(path, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			values, err := LoadAndRepair(dir, CatalogueDefault)
			if invalid == "''" {
				if err != nil {
					t.Fatal("empty identity was not treated as missing", err)
				}
			} else {
				if !errors.Is(err, ErrRepaired) {
					t.Fatal("invalid identity did not use ordinary recovery", err)
				}
				data, err := os.ReadFile(path + ".bak")
				if err != nil || string(data) != source {
					t.Fatal("backup lost original data", err)
				}
			}
			rows := values.Collection(EditorCollection)
			if len(rows) != 2 || rows[0]["id"] != "123" || rows[1]["id"] == "123" || !ValidIdentity(rows[1]["id"].(string)) || rows[1]["path"] != "second.exe" || values.Get(CatalogueDefault, "appearance.autocolor.salt").Number != 42 {
				t.Fatal("repair lost a rule, identity ownership or unrelated setting", rows)
			}
			if err := Save(dir, values); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadAndRepair(dir, CatalogueDefault); err != nil {
				t.Fatal("identity repair repeated", err)
			}
		})
	}
}

func TestIdentityPolicyIsReusableForOtherCollections(t *testing.T) {
	catalogue, err := ParseCatalogue(`[[items]]
uuid = ""
name = ""
`, `[items]
label = "Items"
title = "Items"
identity = "uuid"
[items.uuid]
label = "ID"
visible = false
[items.name]
label = "Name"
`)
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("[[items]]\nname = 'one'\n[[items]]\nuuid = 'user_chosen-123'\nname = 'two'\n")
	values, err := loadDocument(source, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	rows := values.Collection("items")
	if !ValidIdentity(rows[0]["uuid"].(string)) || rows[1]["uuid"] != "user_chosen-123" {
		t.Fatal("generic identity declaration was not applied", rows)
	}
	rows[0]["uuid"] = rows[1]["uuid"]
	values.SetCollection("items", rows)
	if err := catalogue.NormalizeIdentities(&values); !errors.Is(err, ErrValue) {
		t.Fatal("generic duplicate was not reported", err)
	}
	if values.Collection("items")[0]["uuid"] != "user_chosen-123" || values.Collection("items")[1]["uuid"] == "user_chosen-123" {
		t.Fatal("generic repair lost first identity ownership")
	}
}
