package caddyconfig

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/psviderski/uncloud/internal/machine/store"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testController(t *testing.T, socketPath string) *Controller {
	t.Helper()
	group, err := user.LookupGroupId(strconv.Itoa(os.Getegid()))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "Caddyfile")
	client := NewCaddyAdminClient(socketPath)
	return &Controller{
		machineID:     "test-machine-id",
		caddyfilePath: path,
		generator:     NewCaddyfileGenerator("test-machine-id", "test-machine", client, nil),
		client:        client,
		log:           slog.Default(),
		fileGroup:     group.Name,
	}
}

type testCaddyAdmin struct {
	mu                    sync.Mutex
	adaptRequests         []string
	loadCount             int
	rejectAdaptContaining string
	rejectLoad            bool
}

func (a *testCaddyAdmin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/adapt":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		a.mu.Lock()
		a.adaptRequests = append(a.adaptRequests, string(body))
		reject := a.rejectAdaptContaining != "" && strings.Contains(string(body), a.rejectAdaptContaining)
		a.mu.Unlock()
		if reject {
			http.Error(w, "invalid Caddyfile", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"result":{}}`)
	case "/load":
		a.mu.Lock()
		a.loadCount++
		reject := a.rejectLoad
		a.mu.Unlock()
		if reject {
			http.Error(w, "load rejected", http.StatusBadRequest)
		}
	default:
		http.NotFound(w, r)
	}
}

func (a *testCaddyAdmin) snapshot() ([]string, int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.adaptRequests...), a.loadCount
}

func (a *testCaddyAdmin) setRejectLoad(reject bool) {
	a.mu.Lock()
	a.rejectLoad = reject
	a.mu.Unlock()
}

func TestController_GenerateAndLoadCaddyfile(t *testing.T) {
	t.Run("keeps the saved file when there is no local Caddy container", func(t *testing.T) {
		controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
		saved := "# previous full Caddyfile\n"
		require.NoError(t, os.WriteFile(controller.caddyfilePath, []byte(saved), 0o640))
		remote := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{ remote }",
			"other-machine", time.Now())

		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), []store.ContainerRecord{remote}))
		got, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Equal(t, saved, string(got))
	})

	t.Run("does not publish a bootstrap with an invalid global template", func(t *testing.T) {
		controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{{upstreams",
			"test-machine-id", time.Now())

		require.ErrorContains(t, controller.generateAndLoadCaddyfile(context.Background(),
			[]store.ContainerRecord{caddyCtr}), "render template")
		_, err := os.Stat(controller.caddyfilePath)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("bootstraps unhealthy Caddy with its globals and healthy routes", func(t *testing.T) {
		controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage uncloud\n}",
			"test-machine-id", time.Now())
		caddyCtr.Container.State.Health = &container.Health{Status: "unhealthy"}
		newerStopped := newContainerRecordWithCaddyConfig("caddy", "10.210.0.6", "{\n\tstorage replacement\n}",
			"test-machine-id", time.Now().Add(time.Minute))
		newerStopped.Container.State.Running = false
		app := newContainerRecordWithPorts("web", "10.210.0.3", []string{"app.example.com:8080/http"},
			"test-machine-id")
		app.Container.ServiceSpec.Caddy = &api.CaddySpec{Config: "custom.example.com { respond app }"}
		unhealthy := newContainerRecordWithPorts("unhealthy", "10.210.0.4",
			[]string{"unhealthy.example.com:8080/http"}, "test-machine-id")
		unhealthy.Container.State.Health = &container.Health{Status: "unhealthy"}
		hook := newContainerRecordWithPorts("hook", "10.210.0.5",
			[]string{"hook.example.com:8080/http"}, "test-machine-id")
		hook.Container.Config.Labels[api.LabelHook] = "pre-deploy"

		err := controller.generateAndLoadCaddyfile(context.Background(),
			[]store.ContainerRecord{newerStopped, caddyCtr, app, unhealthy, hook})
		require.ErrorContains(t, err, "admin socket unavailable")
		saved, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Contains(t, string(saved), bootstrapMarker+caddyCtr.Container.ID)
		assert.Contains(t, string(saved), "storage uncloud")
		assert.NotContains(t, string(saved), "storage replacement")
		assert.Contains(t, string(saved), "app.example.com")
		assert.NotContains(t, string(saved), "custom.example.com")
		assert.NotContains(t, string(saved), "unhealthy.example.com")
		assert.NotContains(t, string(saved), "hook.example.com")
	})

	t.Run("replaces a legacy offline file with a bootstrap containing globals", func(t *testing.T) {
		controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
		legacy := "# generated by older daemon\nhttp:// { respond ok }\n" + legacyBootstrapMarker + "\n"
		require.NoError(t, os.WriteFile(controller.caddyfilePath, []byte(legacy), 0o640))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage uncloud\n}",
			"test-machine-id", time.Now())

		err := controller.generateAndLoadCaddyfile(context.Background(), []store.ContainerRecord{caddyCtr})
		require.ErrorContains(t, err, "admin socket unavailable")
		saved, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Contains(t, string(saved), "storage uncloud")
		assert.Contains(t, string(saved), bootstrapMarker+caddyCtr.Container.ID)
		assert.NotContains(t, string(saved), legacyBootstrapMarker)
	})

	t.Run("saves a bootstrap for a stopped Caddy container with no file", func(t *testing.T) {
		controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage uncloud\n}",
			"test-machine-id", time.Now())
		caddyCtr.Container.State.Running = false

		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), []store.ContainerRecord{caddyCtr}))
		saved, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Contains(t, string(saved), bootstrapMarker+caddyCtr.Container.ID)
		assert.Contains(t, string(saved), "storage uncloud")
	})

	for _, tc := range []struct {
		name    string
		running bool
	}{
		{name: "preserves a full file when the admin socket is unavailable", running: true},
		{name: "preserves a stopped container's full file", running: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
			saved := "{\n\tstorage uncloud\n}\n\nold.example.com { respond old }\n"
			require.NoError(t, os.WriteFile(controller.caddyfilePath, []byte(saved), 0o640))
			caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage new\n}",
				"test-machine-id", time.Now())
			caddyCtr.Container.State.Running = tc.running
			app := newContainerRecordWithPorts("web", "10.210.0.3", []string{"new.example.com:8080/http"},
				"test-machine-id")

			err := controller.generateAndLoadCaddyfile(context.Background(), []store.ContainerRecord{caddyCtr, app})
			if tc.running {
				require.ErrorContains(t, err, "admin socket unavailable")
			} else {
				require.NoError(t, err)
			}
			got, err := os.ReadFile(controller.caddyfilePath)
			require.NoError(t, err)
			assert.Equal(t, saved, string(got))
		})
	}

	t.Run("loads a full file and reloads after Caddy restarts", func(t *testing.T) {
		admin := &testCaddyAdmin{}
		controller := testController(t, testAdminServer(t, admin.ServeHTTP))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage uncloud\n}",
			"test-machine-id", time.Now())
		caddyCtr.Container.State.Health = &container.Health{Status: "unhealthy"}
		app := newContainerRecordWithCaddyConfig("web", "10.210.0.3", "app.example.com { respond app }",
			"test-machine-id", time.Now())
		records := []store.ContainerRecord{caddyCtr, app}

		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		saved, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Contains(t, string(saved), "storage uncloud")
		assert.Contains(t, string(saved), "app.example.com")
		assert.NotContains(t, string(saved), bootstrapMarker)
		adapted, loads := admin.snapshot()
		require.NotEmpty(t, adapted)
		assert.Contains(t, adapted[0], "storage uncloud")
		assert.Equal(t, 1, loads)

		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		_, loads = admin.snapshot()
		assert.Equal(t, 1, loads, "unchanged inputs should not reload Caddy")

		records[0].Container.State.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		_, loads = admin.snapshot()
		assert.Equal(t, 2, loads, "a restarted Caddy process needs the full configuration")
	})

	t.Run("reloads when an application route changes", func(t *testing.T) {
		admin := &testCaddyAdmin{}
		controller := testController(t, testAdminServer(t, admin.ServeHTTP))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage uncloud\n}",
			"test-machine-id", time.Now())
		app := newContainerRecordWithPorts("web", "10.210.0.3", []string{"app.example.com:8080/http"},
			"test-machine-id")
		records := []store.ContainerRecord{caddyCtr, app}

		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		_, loads := admin.snapshot()
		assert.Equal(t, 1, loads)

		records[1] = newContainerRecordWithPorts("web", "10.210.0.4",
			[]string{"app.example.com:8080/http"}, "test-machine-id")
		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		saved, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Contains(t, string(saved), "10.210.0.4:8080")
		assert.NotContains(t, string(saved), "10.210.0.3:8080")
		_, loads = admin.snapshot()
		assert.Equal(t, 2, loads)
	})

	t.Run("rejects invalid globals without loading or replacing the saved file", func(t *testing.T) {
		admin := &testCaddyAdmin{rejectAdaptContaining: "storage rejected"}
		controller := testController(t, testAdminServer(t, admin.ServeHTTP))
		saved := "{\n\tstorage uncloud\n}\n\nold.example.com { respond old }\n"
		require.NoError(t, os.WriteFile(controller.caddyfilePath, []byte(saved), 0o640))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage rejected\n}",
			"test-machine-id", time.Now())

		err := controller.generateAndLoadCaddyfile(context.Background(), []store.ContainerRecord{caddyCtr})
		require.ErrorContains(t, err, "validate user-defined global Caddy config")
		got, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Equal(t, saved, string(got))
		_, loads := admin.snapshot()
		assert.Zero(t, loads)
	})

	t.Run("skips invalid application custom config", func(t *testing.T) {
		admin := &testCaddyAdmin{rejectAdaptContaining: "invalid.example.com"}
		controller := testController(t, testAdminServer(t, admin.ServeHTTP))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage uncloud\n}",
			"test-machine-id", time.Now())
		invalid := newContainerRecordWithCaddyConfig("invalid", "10.210.0.3",
			"invalid.example.com { respond bad }", "test-machine-id", time.Now())
		valid := newContainerRecordWithCaddyConfig("valid", "10.210.0.4",
			"valid.example.com { respond good }", "test-machine-id", time.Now())

		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(),
			[]store.ContainerRecord{caddyCtr, invalid, valid}))
		saved, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Contains(t, string(saved), "storage uncloud")
		assert.Contains(t, string(saved), "valid.example.com")
		assert.NotContains(t, string(saved), "invalid.example.com")
		assert.Contains(t, string(saved), "Skipped invalid user-defined configs")
		_, loads := admin.snapshot()
		assert.Equal(t, 1, loads)
	})

	t.Run("keeps the saved file when application validation cannot complete", func(t *testing.T) {
		var loads atomic.Int64
		socket := testAdminServer(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/adapt":
				http.Error(w, "adapter unavailable", http.StatusInternalServerError)
			case "/load":
				loads.Add(1)
			default:
				http.NotFound(w, r)
			}
		})
		controller := testController(t, socket)
		saved := "old.example.com { respond old }\n"
		require.NoError(t, os.WriteFile(controller.caddyfilePath, []byte(saved), 0o640))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "",
			"test-machine-id", time.Now())
		app := newContainerRecordWithCaddyConfig("web", "10.210.0.3", "new.example.com { respond new }",
			"test-machine-id", time.Now())

		err := controller.generateAndLoadCaddyfile(context.Background(), []store.ContainerRecord{caddyCtr, app})
		require.ErrorContains(t, err, "validate Caddy config for service")
		got, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Equal(t, saved, string(got))
		assert.Zero(t, loads.Load())
	})

	t.Run("retries a rejected load without changing inputs", func(t *testing.T) {
		admin := &testCaddyAdmin{rejectLoad: true}
		controller := testController(t, testAdminServer(t, admin.ServeHTTP))
		saved := "{\n\tstorage old\n}\n\nold.example.com { respond old }\n"
		require.NoError(t, os.WriteFile(controller.caddyfilePath, []byte(saved), 0o640))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage uncloud\n}",
			"test-machine-id", time.Now())
		records := []store.ContainerRecord{caddyCtr}

		require.ErrorContains(t, controller.generateAndLoadCaddyfile(context.Background(), records), "load Caddyfile")
		got, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Equal(t, saved, string(got))
		admin.setRejectLoad(false)

		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		got, err = os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Contains(t, string(got), "storage uncloud")
		_, loads := admin.snapshot()
		assert.Equal(t, 2, loads)
	})

	t.Run("retries saving after a successful load", func(t *testing.T) {
		admin := &testCaddyAdmin{}
		controller := testController(t, testAdminServer(t, admin.ServeHTTP))
		group := controller.fileGroup
		controller.fileGroup = "uncloud-test-group-that-does-not-exist"
		saved := "{\n\tstorage old\n}\n\nold.example.com { respond old }\n"
		require.NoError(t, os.WriteFile(controller.caddyfilePath, []byte(saved), 0o640))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage uncloud\n}",
			"test-machine-id", time.Now())
		records := []store.ContainerRecord{caddyCtr}

		require.ErrorContains(t, controller.generateAndLoadCaddyfile(context.Background(), records), "save loaded Caddyfile")
		got, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Equal(t, saved, string(got))
		controller.fileGroup = group

		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		got, err = os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Contains(t, string(got), "storage uncloud")
		_, loads := admin.snapshot()
		assert.Equal(t, 2, loads)
		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		_, loads = admin.snapshot()
		assert.Equal(t, 2, loads, "a loaded and saved revision should not reload")
	})

	t.Run("loads a saved bootstrap when the admin socket becomes available", func(t *testing.T) {
		controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
		caddyCtr := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{\n\tstorage uncloud\n}",
			"test-machine-id", time.Now())
		records := []store.ContainerRecord{caddyCtr}
		require.Error(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		bootstrap, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Contains(t, string(bootstrap), bootstrapMarker)

		admin := &testCaddyAdmin{}
		socket := testAdminServer(t, admin.ServeHTTP)
		controller.client = NewCaddyAdminClient(socket)
		controller.generator = NewCaddyfileGenerator("test-machine-id", "test-machine", controller.client, nil)
		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		full, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.NotContains(t, string(full), bootstrapMarker)
		_, loads := admin.snapshot()
		assert.Equal(t, 1, loads)

		require.NoError(t, os.WriteFile(controller.caddyfilePath, bootstrap, 0o640))
		require.NoError(t, controller.generateAndLoadCaddyfile(context.Background(), records))
		_, loads = admin.snapshot()
		assert.Equal(t, 2, loads, "a saved bootstrap is not an applied full configuration")
	})
}

func TestController_WriteCaddyfileIfChanged(t *testing.T) {
	t.Run("publishes a complete file with the configured group", func(t *testing.T) {
		controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
		caddyfile := "# Generated now\n{\n\tstorage uncloud\n}\n"

		require.NoError(t, controller.writeCaddyfileIfChanged(caddyfile))
		got, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Equal(t, caddyfile, string(got))
		info, err := os.Stat(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	})

	t.Run("does not replace a file when only the timestamp changes", func(t *testing.T) {
		controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
		saved := "# Generated yesterday\nsite.example.com { respond ok }\n"
		require.NoError(t, os.WriteFile(controller.caddyfilePath, []byte(saved), 0o640))
		before, err := os.Stat(controller.caddyfilePath)
		require.NoError(t, err)

		require.NoError(t, controller.writeCaddyfileIfChanged(
			"# Generated today\nsite.example.com { respond ok }\n"))
		got, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Equal(t, saved, string(got))
		after, err := os.Stat(controller.caddyfilePath)
		require.NoError(t, err)
		assert.True(t, os.SameFile(before, after), "an unchanged body should not replace the file")
	})

	t.Run("preserves the previous file when publication fails", func(t *testing.T) {
		controller := testController(t, filepath.Join(t.TempDir(), "missing.sock"))
		controller.fileGroup = "uncloud-test-group-that-does-not-exist"
		saved := "# previous\nsite.example.com { respond old }\n"
		require.NoError(t, os.WriteFile(controller.caddyfilePath, []byte(saved), 0o640))

		err := controller.writeCaddyfileIfChanged("# next\nsite.example.com { respond new }\n")
		require.ErrorContains(t, err, "change owner of temporary Caddyfile")
		got, err := os.ReadFile(controller.caddyfilePath)
		require.NoError(t, err)
		assert.Equal(t, saved, string(got))
		tmpFiles, err := filepath.Glob(filepath.Join(filepath.Dir(controller.caddyfilePath), ".Caddyfile-*"))
		require.NoError(t, err)
		assert.Empty(t, tmpFiles)
	})
}

func TestSelectLocalCaddyContainer(t *testing.T) {
	t.Run("prefers a running unhealthy container over a newer stopped one", func(t *testing.T) {
		older := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{ older }",
			"test-machine-id", time.Now().Add(-time.Hour))
		older.Container.State.Health = &container.Health{Status: "unhealthy"}
		newer := newContainerRecordWithCaddyConfig("caddy", "10.210.0.3", "{ newer }",
			"test-machine-id", time.Now())
		newer.Container.State.Running = false

		selected := selectLocalCaddyContainer([]store.ContainerRecord{newer, older}, "test-machine-id")
		require.NotNil(t, selected)
		assert.Equal(t, older.Container.ID, selected.ID)
	})

	t.Run("ignores remote and hook containers", func(t *testing.T) {
		local := newContainerRecordWithCaddyConfig("caddy", "10.210.0.2", "{ local }",
			"test-machine-id", time.Now().Add(-time.Hour))
		remote := newContainerRecordWithCaddyConfig("caddy", "10.210.0.3", "{ remote }",
			"other-machine", time.Now())
		hook := newContainerRecordWithCaddyConfig("caddy", "10.210.0.4", "{ hook }",
			"test-machine-id", time.Now())
		hook.Container.Config.Labels[api.LabelHook] = "pre-deploy"

		selected := selectLocalCaddyContainer([]store.ContainerRecord{remote, hook, local}, "test-machine-id")
		require.NotNil(t, selected)
		assert.Equal(t, local.Container.ID, selected.ID)
	})
}

// TestContainerFingerprint_EqualCoversAllFields is a guard: when a field is added to containerFingerprint,
// Equal must also compare it. A mutation of any single field should flip equality to false. If this test fails
// after adding a field, update Equal to include it.
func TestContainerFingerprint_EqualCoversAllFields(t *testing.T) {
	t.Parallel()

	base := containerFingerprint{
		ID: "container-1",
		IP: netip.MustParseAddr("10.210.0.2"),
		Ports: []api.PortSpec{{
			Hostname:      "app.example.com",
			ContainerPort: 8080,
			Protocol:      api.ProtocolHTTP,
			Mode:          api.PortModeIngress,
		}},
		CaddyConfig: "caddy-config",
	}

	assert.True(t, base.Equal(base), "base fingerprint must be equal to itself")

	rt := reflect.TypeOf(base)
	for field := range rt.Fields() {
		t.Run(field.Name, func(t *testing.T) {
			t.Parallel()

			mutated := base
			mutated.Ports = append([]api.PortSpec(nil), base.Ports...)
			v := reflect.ValueOf(&mutated).Elem().FieldByName(field.Name)

			switch field.Name {
			case "ID", "CaddyConfig":
				v.SetString(v.String() + "-changed")
			case "IP":
				v.Set(reflect.ValueOf(netip.MustParseAddr("10.210.0.99")))
			case "Ports":
				mutated.Ports = []api.PortSpec{{
					Hostname:      "different.example.com",
					ContainerPort: 9090,
					Protocol:      api.ProtocolHTTP,
					Mode:          api.PortModeIngress,
				}}
			default:
				t.Fatalf("containerFingerprint has a new field %q without a mutation case in this test. "+
					"Add a case here and make sure Equal() compares it.", field.Name)
			}

			assert.False(t, base.Equal(mutated),
				"changing %q must flip Equal to false. Update containerFingerprint.Equal to compare it.",
				field.Name)
		})
	}
}
