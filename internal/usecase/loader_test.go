package usecase

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iurykrieger/lastro/internal/usecase/internal/fixturestub"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

func TestLoadValidYAML(t *testing.T) {
	data := []byte(`schema_version: 2.0.0
id: load-test-uc
title: "Loader smoke"
archetype_scope: [http-api]
entry_points:
  - id: ep-x
    archetype: http-api
    spec:
      method: POST
      path: /x
given:
  - "given ${{fixtures.fx-x}}"
when:
  - "when ${{entry_points.ep-x}}"
then:
  - "then ok"
fixture_ids: [fx-x]
`)
	store := fixturestub.New(map[string]string{"fx-x": `{"a":1}`})
	uc, err := Load(data, store)
	if err != nil {
		t.Fatalf("Load err: %v", err)
	}
	if uc.ID != "load-test-uc" {
		t.Errorf("ID: %q", uc.ID)
	}
	if len(uc.GivenSegments(0)) == 0 {
		t.Error("expected cached given segments")
	}
}

func TestLoadRejectsUnsupportedSchemaVersion(t *testing.T) {
	data := []byte(`schema_version: 9.9.9
id: x
title: t
archetype_scope: [http-api]
entry_points:
  - id: e
    archetype: http-api
    spec: { method: GET, path: / }
given: [g]
when: [w]
then: [t]
`)
	_, err := Load(data, fixturestub.New(nil))
	if !hasCode(err, "USECASE_SCHEMA_VERSION_UNSUPPORTED") {
		t.Errorf("want USECASE_SCHEMA_VERSION_UNSUPPORTED, got %v", err)
	}
}

func TestLoadAllGoldenUseCases(t *testing.T) {
	cases := []struct {
		file     string
		fixtures map[string]string
	}{
		{"use-case/batch-job.yaml", map[string]string{
			"orders-csv-fixture":     `{}`,
			"orders-db-rows-fixture": `{}`,
		}},
		{"use-case/cli.yaml", map[string]string{
			"detect-args-fixture":   `{"args":["detect"]}`,
			"detect-stdout-fixture": `{"stack":[]}`,
		}},
		{"use-case/event-consumer.yaml", map[string]string{
			"order-created-event-fixture": `{"order_id":"o-1"}`,
		}},
		{"use-case/event-producer.yaml", map[string]string{
			"charge-success-state-fixture":    `{}`,
			"payment-processed-event-fixture": `{}`,
		}},
		{"use-case/http-api.yaml", map[string]string{
			"order-input-fixture":  `{"customer_id":"c-001"}`,
			"order-output-fixture": `{"order_id":"o-1"}`,
		}},
		{"use-case/library.yaml", map[string]string{
			"config-input-fixture":  `{}`,
			"config-parsed-fixture": `{}`,
		}},
		{"use-case/sdk.yaml", map[string]string{
			"client-config-fixture":   `{}`,
			"client-instance-fixture": `{}`,
		}},
		{"use-case/static-site.yaml", map[string]string{
			"rate-card-fixture":    `{}`,
			"pricing-html-fixture": `{}`,
		}},
		{"use-case/worker.yaml", map[string]string{
			"ledger-state-fixture":          `{}`,
			"reconciliation-result-fixture": `{}`,
		}},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			path := filepath.Join("..", "..", "schemas", "examples", c.file)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			uc, err := Load(data, fixturestub.New(c.fixtures))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if uc.ID == "" {
				t.Error("UseCase.ID empty after load")
			}
		})
	}
}

func TestRenderLabelsForHTTPAPIGolden(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "examples", "use-case", "http-api.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	store := fixturestub.New(map[string]string{
		"order-input-fixture":  `{"customer_id":"c-001"}`,
		"order-output-fixture": `{"order_id":"o-1"}`,
	})
	uc, err := Load(data, store)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	givenLabel := template.RenderLabels(uc.GivenSegments(0))
	wantGiven := "A request payload matching [fixture: order-input-fixture] is constructed by the client"
	if givenLabel != wantGiven {
		t.Errorf("given[0] label\n got: %q\nwant: %q", givenLabel, wantGiven)
	}
	whenLabel := template.RenderLabels(uc.WhenSegments(0))
	wantWhen := "The client invokes [entry: create-order-endpoint]"
	if whenLabel != wantWhen {
		t.Errorf("when[0] label\n got: %q\nwant: %q", whenLabel, wantWhen)
	}
}
