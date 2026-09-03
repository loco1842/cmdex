package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type recordingLauncherHotkeyManager struct {
	mu              sync.Mutex
	registers       int
	unregisters     int
	registered      bool
	shortcut        string
	callback        func()
	registrationErr error
	unregisterErr   error
	registerStarted chan struct{}
	registerRelease chan struct{}
	registerOnce    sync.Once
}

func (m *recordingLauncherHotkeyManager) Register(shortcut string, callback func()) error {
	if m.registerStarted != nil {
		m.registerOnce.Do(func() { close(m.registerStarted) })
		<-m.registerRelease
	}
	m.mu.Lock()
	m.registers++
	err := m.registrationErr
	if err == nil {
		m.registered = true
		m.shortcut = shortcut
		m.callback = callback
	}
	m.mu.Unlock()
	return err
}

func (m *recordingLauncherHotkeyManager) Unregister(_ string) error {
	m.mu.Lock()
	m.unregisters++
	err := m.unregisterErr
	if err == nil {
		m.registered = false
	}
	m.mu.Unlock()
	return err
}

func (m *recordingLauncherHotkeyManager) IsRegistered(_ string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registered
}

func (m *recordingLauncherHotkeyManager) Validate(shortcut string) error {
	if !strings.Contains(shortcut, "+") {
		return fmt.Errorf("shortcut %q needs at least one modifier", shortcut)
	}
	return nil
}

func (m *recordingLauncherHotkeyManager) registerCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registers
}

func (m *recordingLauncherHotkeyManager) unregisterCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.unregisters
}

func (m *recordingLauncherHotkeyManager) invoke() {
	m.mu.Lock()
	callback := m.callback
	m.mu.Unlock()
	if callback != nil {
		callback()
	}
}

func launcherTestDB(t *testing.T) *DB {
	t.Helper()
	previousDB := db
	testDB := newTestDB(t)
	if err := testDB.runMigrations(); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}
	db = testDB
	t.Cleanup(func() {
		db = previousDB
		_ = testDB.Close()
	})
	return testDB
}

func useLauncherHotkeyManager(t *testing.T, manager launcherHotkeyManager) {
	t.Helper()
	previousFactory := newLauncherHotkeyManager
	newLauncherHotkeyManager = func() launcherHotkeyManager { return manager }
	t.Cleanup(func() { newLauncherHotkeyManager = previousFactory })
}

func TestFreshSettingsDisableLauncher(t *testing.T) {
	testDB := launcherTestDB(t)

	settings, err := testDB.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings failed: %v", err)
	}
	if settings.LauncherEnabled == nil {
		t.Fatal("fresh settings LauncherEnabled is nil, want false")
	}
	if *settings.LauncherEnabled {
		t.Fatal("fresh settings LauncherEnabled = true, want false")
	}
}

func TestLauncherMacCollectionBehaviorSupportsFullscreenAcrossSpaces(t *testing.T) {
	if launcherMacCollectionBehavior&application.MacWindowCollectionBehaviorCanJoinAllSpaces == 0 {
		t.Fatal("launcher collection behavior must join all Spaces")
	}

	want := application.MacWindowCollectionBehaviorCanJoinAllSpaces |
		application.MacWindowCollectionBehaviorFullScreenAuxiliary |
		application.MacWindowCollectionBehaviorIgnoresCycle |
		application.MacWindowCollectionBehaviorStationary
	if launcherMacCollectionBehavior != want {
		t.Fatalf("launcher collection behavior = %d, want %d", launcherMacCollectionBehavior, want)
	}
}

func TestLauncherUsesNonActivatingPanelPolicy(t *testing.T) {
	mac := launcherMacOptions()
	if mac.WindowClass != application.MacWindowClassPanel {
		t.Fatalf("launcher window class = %v, want NSPanel", mac.WindowClass)
	}
	if !mac.PanelPreferences.NonActivating {
		t.Fatal("launcher panel must be non-activating")
	}
	if mac.PanelPreferences.BecomesKeyOnlyIfNeeded {
		t.Fatal("launcher panel must become key when explicitly focused")
	}
	if !mac.PanelPreferences.FloatingPanel {
		t.Fatal("launcher panel must use floating-panel behavior")
	}
	if mac.CollectionBehavior != launcherMacCollectionBehavior {
		t.Fatalf("launcher collection behavior = %d, want %d", mac.CollectionBehavior, launcherMacCollectionBehavior)
	}
}

func TestSettingsPreserveExplicitLauncherValues(t *testing.T) {
	testDB := launcherTestDB(t)
	enabled := true
	if err := testDB.SetSettings(AppSettings{LauncherEnabled: &enabled}); err != nil {
		t.Fatalf("enable launcher: %v", err)
	}
	settings, err := testDB.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings after enable failed: %v", err)
	}
	if settings.LauncherEnabled == nil || !*settings.LauncherEnabled {
		t.Fatal("explicit true launcher setting was not persisted")
	}

	disabled := false
	if err := testDB.SetSettings(AppSettings{LauncherEnabled: &disabled}); err != nil {
		t.Fatalf("disable launcher: %v", err)
	}
	settings, err = testDB.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings after disable failed: %v", err)
	}
	if settings.LauncherEnabled == nil || *settings.LauncherEnabled {
		t.Fatal("explicit false launcher setting was not persisted")
	}
}

func TestLauncherServiceApplySettingsTreatsNilAsDisabled(t *testing.T) {
	testDB := launcherTestDB(t)
	if _, err := testDB.conn.Exec(`UPDATE app_settings SET data = '{"launcherEnabled":null}'`); err != nil {
		t.Fatalf("seed nil launcher setting: %v", err)
	}

	manager := &recordingLauncherHotkeyManager{}
	service := &LauncherService{hotkeys: manager}
	status := service.ApplySettings()
	if status.Enabled {
		t.Fatal("nil LauncherEnabled produced an enabled status")
	}
	if status.Registered {
		t.Fatal("nil LauncherEnabled registered a global shortcut")
	}
	if got := manager.registerCount(); got != 0 {
		t.Fatalf("Register called %d times for nil LauncherEnabled, want 0", got)
	}
}

func TestLauncherServiceApplySettingsExplicitEnableRegisters(t *testing.T) {
	testDB := launcherTestDB(t)
	enabled := true
	if err := testDB.SetSettings(AppSettings{LauncherEnabled: &enabled}); err != nil {
		t.Fatalf("enable launcher: %v", err)
	}

	manager := &recordingLauncherHotkeyManager{}
	service := &LauncherService{hotkeys: manager}
	status := service.ApplySettings()
	if !status.Enabled {
		t.Fatal("explicit true LauncherEnabled produced a disabled status")
	}
	if !status.Registered {
		t.Fatal("explicit true LauncherEnabled did not register the shortcut")
	}
	if got := manager.registerCount(); got != 1 {
		t.Fatalf("Register called %d times for explicit true LauncherEnabled, want 1", got)
	}
}

func TestLauncherServiceApplySettingsReportsUnregisterFailure(t *testing.T) {
	testDB := launcherTestDB(t)
	enabled := true
	if err := testDB.SetSettings(AppSettings{LauncherEnabled: &enabled}); err != nil {
		t.Fatalf("enable launcher: %v", err)
	}

	manager := &recordingLauncherHotkeyManager{
		registered:    true,
		shortcut:      "Ctrl+Shift+K",
		unregisterErr: errors.New("hotkey manager unavailable"),
	}
	service := &LauncherService{
		hotkeys:            manager,
		registeredShortcut: "Ctrl+Shift+K",
	}

	status := service.ApplySettings()
	if !strings.Contains(status.Error, "unregister shortcut") {
		t.Fatalf("ApplySettings error = %q, want unregister failure", status.Error)
	}
	if !status.Registered {
		t.Fatalf("ApplySettings status = %+v, want the still-registered shortcut reported", status)
	}
	if got := manager.registerCount(); got != 0 {
		t.Fatalf("Register called %d times after unregister failure, want 0", got)
	}
	if service.registeredShortcut != "Ctrl+Shift+K" {
		t.Fatalf("registeredShortcut = %q, want retained ownership marker", service.registeredShortcut)
	}
}

func TestLauncherServiceApplySettingsUsesAutostartProvider(t *testing.T) {
	testDB := launcherTestDB(t)
	enabled := false
	if err := testDB.SetSettings(AppSettings{LauncherEnabled: &enabled}); err != nil {
		t.Fatalf("disable launcher: %v", err)
	}

	service := &LauncherService{
		hotkeys:            &recordingLauncherHotkeyManager{},
		autostartEnabledFn: func() bool { return true },
	}
	status := service.ApplySettings()
	if !status.LaunchAtLogin {
		t.Fatalf("ApplySettings LaunchAtLogin = false, want provider value true")
	}
}

func TestLauncherServiceApplySettingsDoesNotOverwriteConcurrentLoginToggle(t *testing.T) {
	testDB := launcherTestDB(t)
	enabled := false
	if err := testDB.SetSettings(AppSettings{LauncherEnabled: &enabled}); err != nil {
		t.Fatalf("disable launcher: %v", err)
	}

	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	var providerOnce sync.Once
	osEnabled := false
	var stateMu sync.Mutex
	service := &LauncherService{
		hotkeys: &recordingLauncherHotkeyManager{},
		autostartEnabledFn: func() bool {
			providerOnce.Do(func() { close(providerStarted) })
			<-releaseProvider
			stateMu.Lock()
			defer stateMu.Unlock()
			return osEnabled
		},
		setAutostartFn: func(value bool) error {
			stateMu.Lock()
			osEnabled = value
			stateMu.Unlock()
			return nil
		},
		persistLoginFn: func(bool) error { return nil },
	}

	applyResult := make(chan LauncherStatus, 1)
	go func() { applyResult <- service.ApplySettings() }()
	select {
	case <-providerStarted:
	case <-time.After(time.Second):
		t.Fatal("ApplySettings did not read the autostart provider")
	}

	toggleResult := make(chan error, 1)
	go func() { toggleResult <- service.SetLaunchAtLogin(true) }()
	deadline := time.Now().Add(time.Second)
	for atomic.LoadUint64(&service.loginOp) == 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if atomic.LoadUint64(&service.loginOp) != 1 {
		t.Fatal("concurrent launch-at-login toggle did not start")
	}
	close(releaseProvider)

	select {
	case <-applyResult:
	case <-time.After(time.Second):
		t.Fatal("ApplySettings did not complete")
	}
	select {
	case err := <-toggleResult:
		if err != nil {
			t.Fatalf("SetLaunchAtLogin failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SetLaunchAtLogin did not complete")
	}
	if status := service.GetStatus(); !status.LaunchAtLogin {
		t.Fatalf("final LaunchAtLogin = false, want concurrent toggle to win: %+v", status)
	}
}

func TestLauncherServiceApplySettingsReregistersAndShutdownUnregisters(t *testing.T) {
	testDB := launcherTestDB(t)
	enabled := true
	if err := testDB.SetSettings(AppSettings{LauncherEnabled: &enabled}); err != nil {
		t.Fatalf("enable launcher: %v", err)
	}

	manager := &recordingLauncherHotkeyManager{}
	service := &LauncherService{hotkeys: manager}
	if status := service.ApplySettings(); !status.Registered {
		t.Fatalf("initial ApplySettings status = %+v, want registered", status)
	}
	if status := service.ApplySettings(); !status.Registered {
		t.Fatalf("second ApplySettings status = %+v, want registered", status)
	}
	if got := manager.registerCount(); got != 2 {
		t.Fatalf("Register called %d times, want 2", got)
	}
	if got := manager.unregisterCount(); got != 1 {
		t.Fatalf("Unregister called %d times before shutdown, want 1", got)
	}

	if err := service.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown failed: %v", err)
	}
	if got := manager.unregisterCount(); got != 2 {
		t.Fatalf("Unregister called %d times after shutdown, want 2", got)
	}
}

func TestLauncherServiceApplySettingsReportsUnsupportedManager(t *testing.T) {
	testDB := launcherTestDB(t)
	enabled := true
	if err := testDB.SetSettings(AppSettings{LauncherEnabled: &enabled}); err != nil {
		t.Fatalf("enable launcher: %v", err)
	}

	manager := &recordingLauncherHotkeyManager{
		registrationErr: errors.New("global shortcuts are not supported on this platform"),
	}
	service := &LauncherService{hotkeys: manager}
	status := service.ApplySettings()
	if status.Supported {
		t.Fatalf("unsupported manager status = %+v, want Supported=false", status)
	}
	if status.Registered {
		t.Fatalf("unsupported manager status = %+v, want Registered=false", status)
	}
	if status.Error == "" {
		t.Fatal("unsupported manager returned no status error")
	}
}

func TestLauncherServiceValidateShortcutUsesManagerValidator(t *testing.T) {
	manager := &recordingLauncherHotkeyManager{}
	service := &LauncherService{hotkeys: manager}
	if err := service.ValidateShortcut("Ctrl+Shift+K"); err != nil {
		t.Fatalf("valid shortcut rejected: %v", err)
	}
	if err := service.ValidateShortcut("K"); err == nil {
		t.Fatal("bare key accepted as a global shortcut")
	}
}

func TestLauncherServiceStartupDisabledDoesNotRegister(t *testing.T) {
	launcherTestDB(t)
	manager := &recordingLauncherHotkeyManager{}
	useLauncherHotkeyManager(t, manager)

	previousWailsApp := wailsApp
	previousTerminalSvc := terminalSvc
	wailsApp = nil
	terminalSvc = nil
	t.Cleanup(func() {
		wailsApp = previousWailsApp
		terminalSvc = previousTerminalSvc
	})

	service := &LauncherService{}
	if err := service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup failed: %v", err)
	}
	t.Cleanup(func() { _ = service.ServiceShutdown() })

	deadline := time.Now().Add(time.Second)
	for service.GetStatus().Platform == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := manager.registerCount(); got != 0 {
		t.Fatalf("startup called Register %d times with launcher disabled, want 0", got)
	}
	if status := service.GetStatus(); status.Enabled || status.Registered {
		t.Fatalf("startup status = %+v, want disabled and unregistered", status)
	}
}

func TestLauncherServiceShutdownUnregistersStartupRegistration(t *testing.T) {
	testDB := launcherTestDB(t)
	enabled := true
	if err := testDB.SetSettings(AppSettings{LauncherEnabled: &enabled}); err != nil {
		t.Fatalf("enable launcher: %v", err)
	}

	manager := &recordingLauncherHotkeyManager{
		registerStarted: make(chan struct{}),
		registerRelease: make(chan struct{}),
	}
	useLauncherHotkeyManager(t, manager)

	previousWailsApp := wailsApp
	previousTerminalSvc := terminalSvc
	wailsApp = nil
	terminalSvc = nil
	t.Cleanup(func() {
		wailsApp = previousWailsApp
		terminalSvc = previousTerminalSvc
	})

	service := &LauncherService{}
	if err := service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup failed: %v", err)
	}

	select {
	case <-manager.registerStarted:
	case <-time.After(time.Second):
		t.Fatal("startup ApplySettings did not attempt shortcut registration")
	}
	shutdownDone := make(chan struct{})
	go func() {
		_ = service.ServiceShutdown()
		close(shutdownDone)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		service.startupMu.Lock()
		stopping := service.startupShuttingDown
		service.startupMu.Unlock()
		if stopping {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ServiceShutdown did not close startup admission")
		}
		runtime.Gosched()
	}
	close(manager.registerRelease)

	select {
	case <-shutdownDone:
	case <-time.After(time.Second):
		t.Fatal("ServiceShutdown did not join startup work")
	}
	if manager.IsRegistered("") {
		t.Fatal("ServiceShutdown left the startup shortcut registered")
	}
}

func TestLauncherServiceStartupEagerlyCreatesOneSession(t *testing.T) {
	launcherTestDB(t)
	manager := &recordingLauncherHotkeyManager{}
	useLauncherHotkeyManager(t, manager)

	previousWailsApp := wailsApp
	previousTerminalSvc := terminalSvc
	wailsApp = nil
	terminalSvc = nil
	t.Cleanup(func() {
		wailsApp = previousWailsApp
		terminalSvc = previousTerminalSvc
	})

	var mu sync.Mutex
	creates := 0
	service := &LauncherService{
		createSessionFn: func() (*SessionInfo, error) {
			mu.Lock()
			creates++
			mu.Unlock()
			return &SessionInfo{ID: "launcher-eager", Name: "Launcher", Running: true}, nil
		},
		sessionExistsFn: func(id string) bool { return id == "launcher-eager" },
	}
	if err := service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup failed: %v", err)
	}
	t.Cleanup(func() { _ = service.ServiceShutdown() })

	deadline := time.Now().Add(time.Second)
	for {
		id, err := service.GetSessionID()
		if err == nil {
			if id != "launcher-eager" {
				t.Fatalf("eager session ID = %q", id)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("eager session did not become available: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if creates != 1 {
		t.Fatalf("eager startup created %d sessions, want 1", creates)
	}
}

func TestLauncherServiceApplySettingsWaitsForPreviousApplication(t *testing.T) {
	previousDB := db
	testDB := newTestDB(t)
	if err := testDB.runMigrations(); err != nil {
		t.Fatalf("runMigrations failed: %v", err)
	}
	db = testDB
	t.Cleanup(func() {
		db = previousDB
		_ = testDB.Close()
	})

	service := &LauncherService{}
	service.applyMu.Lock()
	result := make(chan LauncherStatus, 1)
	go func() { result <- service.ApplySettings() }()

	select {
	case <-result:
		t.Fatal("ApplySettings completed while another settings application was active")
	case <-time.After(25 * time.Millisecond):
	}

	service.applyMu.Unlock()
	select {
	case <-result:
	case <-time.After(time.Second):
		t.Fatal("ApplySettings did not proceed after the previous application completed")
	}

	// Startup and frontend settings changes can arrive concurrently. Exercise
	// that path repeatedly so the race-enabled test also covers status writes
	// while applications are queued behind applyMu.
	const concurrentCalls = 32
	var wg sync.WaitGroup
	statuses := make(chan LauncherStatus, concurrentCalls)
	for i := 0; i < concurrentCalls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- service.ApplySettings()
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status.Platform == "" {
			t.Error("concurrent ApplySettings returned an empty platform")
		}
	}
}

func TestLauncherServiceApplyStatusKeepsNewerSessionError(t *testing.T) {
	service := &LauncherService{sessionErr: "launcher terminal unavailable: newer failure"}
	status := service.setStatusFromApply(
		LauncherStatus{Error: "launcher terminal unavailable: old failure"},
		"launcher terminal unavailable: old failure",
	)
	if status.Error != service.sessionErr {
		t.Fatalf("ApplySettings status error = %q, want newer session error %q", status.Error, service.sessionErr)
	}
	if got := service.GetStatus().Error; got != service.sessionErr {
		t.Fatalf("stored status error = %q, want newer session error %q", got, service.sessionErr)
	}
}

func TestLauncherServiceApplyStatusPreservesIndependentErrorWhenSessionRecovers(t *testing.T) {
	service := &LauncherService{}
	status := service.setStatusFromApply(
		LauncherStatus{Error: "shortcut registration failed"},
		"launcher terminal unavailable: old failure",
	)
	if status.Error != "shortcut registration failed" {
		t.Fatalf("ApplySettings status error = %q, want independent registration error", status.Error)
	}
}

func TestLauncherServiceToggleTargetIsAtomic(t *testing.T) {
	service := &LauncherService{}
	const toggles = 32
	var wg sync.WaitGroup
	for i := 0; i < toggles; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.nextVisibilityTarget()
		}()
	}
	wg.Wait()

	service.visibilityMu.Lock()
	defer service.visibilityMu.Unlock()
	if service.visibilityTarget {
		t.Fatal("final visibility target is visible after an even number of toggles")
	}
	if service.visibilityOp != toggles {
		t.Fatalf("visibility operation count = %d, want %d", service.visibilityOp, toggles)
	}
}

func TestLauncherServiceSetLaunchAtLoginSkipsSupersededRequest(t *testing.T) {
	for _, tc := range []struct {
		name  string
		first bool
		newer bool
	}{
		{name: "true to false", first: true, newer: false},
		{name: "false to true", first: false, newer: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var callsMu sync.Mutex
			osState := !tc.first
			persistedState := osState
			var autostartCalls []bool
			var persistedCalls []bool
			firstStarted := make(chan struct{})
			releaseFirst := make(chan struct{})
			var firstCall sync.Once

			service := &LauncherService{
				autostartEnabledFn: func() bool {
					callsMu.Lock()
					defer callsMu.Unlock()
					return osState
				},
				persistedLaunchAtLoginFn: func() (bool, error) {
					callsMu.Lock()
					defer callsMu.Unlock()
					return persistedState, nil
				},
				setAutostartFn: func(enabled bool) error {
					callsMu.Lock()
					osState = enabled
					autostartCalls = append(autostartCalls, enabled)
					callsMu.Unlock()
					if enabled == tc.first {
						firstCall.Do(func() { close(firstStarted) })
						<-releaseFirst
					}
					return nil
				},
				persistLoginFn: func(enabled bool) error {
					callsMu.Lock()
					persistedState = enabled
					persistedCalls = append(persistedCalls, enabled)
					callsMu.Unlock()
					return nil
				},
			}

			firstResult := make(chan error, 1)
			go func() { firstResult <- service.SetLaunchAtLogin(tc.first) }()
			select {
			case <-firstStarted:
			case <-time.After(time.Second):
				t.Fatal("first launch-at-login request did not start")
			}

			secondResult := make(chan error, 1)
			go func() { secondResult <- service.SetLaunchAtLogin(tc.newer) }()
			deadline := time.Now().Add(time.Second)
			for atomic.LoadUint64(&service.loginOp) < 2 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if got := atomic.LoadUint64(&service.loginOp); got != 2 {
				t.Fatalf("login operation generation = %d, want newer request generation 2", got)
			}
			close(releaseFirst)

			if err := <-firstResult; err != nil {
				t.Fatalf("superseded launch-at-login request = %v, want successful no-op", err)
			}
			if err := <-secondResult; err != nil {
				t.Fatalf("newer launch-at-login request failed: %v", err)
			}

			callsMu.Lock()
			defer callsMu.Unlock()
			if osState != tc.newer || persistedState != tc.newer {
				t.Fatalf("final launch-at-login state = os:%t persisted:%t, want %t", osState, persistedState, tc.newer)
			}
			if status := service.GetStatus(); status.LaunchAtLogin != tc.newer {
				t.Fatalf("final status LaunchAtLogin = %t, want %t", status.LaunchAtLogin, tc.newer)
			}
			if want := []bool{tc.first, tc.newer}; !reflect.DeepEqual(autostartCalls, want) {
				t.Fatalf("autostart calls = %v, want serialized requests %v", autostartCalls, want)
			}
			if want := []bool{tc.newer}; !reflect.DeepEqual(persistedCalls, want) {
				t.Fatalf("persisted launch-at-login calls = %v, want only newer request %v", persistedCalls, want)
			}
		})
	}
}

func TestLauncherServiceSetLaunchAtLoginReportsAutostartFailure(t *testing.T) {
	wantErr := errors.New("autostart unavailable")
	service := &LauncherService{
		persistedLaunchAtLoginFn: func() (bool, error) { return false, nil },
		autostartEnabledFn:       func() bool { return false },
		setAutostartFn:           func(bool) error { return wantErr },
	}

	if err := service.SetLaunchAtLogin(true); !errors.Is(err, wantErr) {
		t.Fatalf("SetLaunchAtLogin error = %v, want %v", err, wantErr)
	}
}

func TestLauncherServiceSetLaunchAtLoginRollbackUsesPersistedStateAfterSupersededMutation(t *testing.T) {
	var callsMu sync.Mutex
	osState := false
	persistedState := false
	var autostartCalls []bool
	var persistedCalls []bool
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var firstCall sync.Once
	persistErr := errors.New("persist newer launch-at-login setting")

	service := &LauncherService{
		persistedLaunchAtLoginFn: func() (bool, error) {
			callsMu.Lock()
			defer callsMu.Unlock()
			return persistedState, nil
		},
		autostartEnabledFn: func() bool {
			callsMu.Lock()
			defer callsMu.Unlock()
			return osState
		},
		setAutostartFn: func(enabled bool) error {
			callsMu.Lock()
			osState = enabled
			autostartCalls = append(autostartCalls, enabled)
			callsMu.Unlock()
			if enabled {
				firstCall.Do(func() { close(firstStarted) })
				<-releaseFirst
			}
			return nil
		},
		persistLoginFn: func(enabled bool) error {
			callsMu.Lock()
			persistedCalls = append(persistedCalls, enabled)
			callsMu.Unlock()
			return persistErr
		},
	}

	firstResult := make(chan error, 1)
	go func() { firstResult <- service.SetLaunchAtLogin(true) }()
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first launch-at-login request did not start")
	}

	secondResult := make(chan error, 1)
	go func() { secondResult <- service.SetLaunchAtLogin(false) }()
	deadline := time.Now().Add(time.Second)
	for atomic.LoadUint64(&service.loginOp) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := atomic.LoadUint64(&service.loginOp); got != 2 {
		t.Fatalf("login operation generation = %d, want newer request generation 2", got)
	}
	close(releaseFirst)

	if err := <-firstResult; err != nil {
		t.Fatalf("superseded launch-at-login request = %v, want successful no-op", err)
	}
	if err := <-secondResult; !errors.Is(err, persistErr) {
		t.Fatalf("newer launch-at-login error = %v, want persistence error %v", err, persistErr)
	}

	callsMu.Lock()
	defer callsMu.Unlock()
	if osState != persistedState {
		t.Fatalf("rollback left OS state %t and persisted state %t inconsistent", osState, persistedState)
	}
	if want := []bool{true, false, false}; !reflect.DeepEqual(autostartCalls, want) {
		t.Fatalf("autostart calls = %v, want mutation plus rollback to persisted state %v", autostartCalls, want)
	}
	if want := []bool{false}; !reflect.DeepEqual(persistedCalls, want) {
		t.Fatalf("persisted calls = %v, want only newer request %v", persistedCalls, want)
	}
}

func TestLauncherSessionEagerCreationReusesOneSession(t *testing.T) {
	var creates int
	service := &LauncherService{
		createSessionFn: func() (*SessionInfo, error) {
			creates++
			return &SessionInfo{ID: "launcher-1", Name: "Launcher", Running: true}, nil
		},
		sessionExistsFn: func(id string) bool { return id == "launcher-1" },
		sessionShutdown: make(chan struct{}),
	}

	first, err := service.GetSessionID()
	if err != nil {
		t.Fatalf("first GetSessionID failed: %v", err)
	}
	second, err := service.GetSessionID()
	if err != nil {
		t.Fatalf("second GetSessionID failed: %v", err)
	}
	if first != "launcher-1" || second != first {
		t.Fatalf("session IDs = %q, %q, want the same precreated ID", first, second)
	}
	if creates != 1 {
		t.Fatalf("createSession called %d times, want 1", creates)
	}
}

func TestLauncherSessionRejectsEmptyID(t *testing.T) {
	service := &LauncherService{
		createSessionFn: func() (*SessionInfo, error) {
			return &SessionInfo{Name: "Launcher", Running: true}, nil
		},
		sessionShutdown: make(chan struct{}),
	}

	if id, err := service.GetSessionID(); err == nil || id != "" || !strings.Contains(err.Error(), "empty session ID") {
		t.Fatalf("GetSessionID = %q, %v, want empty-session-ID error", id, err)
	}
	if status := service.GetStatus(); !strings.Contains(status.Error, "empty session ID") {
		t.Fatalf("status = %+v, want empty-session-ID error", status)
	}
	service.sessionMu.Lock()
	defer service.sessionMu.Unlock()
	if service.sessionID != "" {
		t.Fatalf("sessionID = %q, want no session recorded", service.sessionID)
	}
}

func TestLauncherSessionTimeoutDetachesFlightAndClosesLateSuccess(t *testing.T) {
	firstRelease := make(chan struct{})
	firstStarted := make(chan struct{})
	var creates atomic.Int32
	closed := make(chan string, 1)

	service := &LauncherService{
		createSessionFn: func() (*SessionInfo, error) {
			if creates.Add(1) == 1 {
				close(firstStarted)
				<-firstRelease
				return &SessionInfo{ID: "late-session"}, nil
			}
			return &SessionInfo{ID: "recovered-session"}, nil
		},
		sessionExistsFn: func(id string) bool { return id == "recovered-session" },
		closeSessionFn: func(id string) error {
			closed <- id
			return nil
		},
		sessionShutdown: make(chan struct{}),
	}

	if _, err := service.ensureSession(10 * time.Millisecond); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timed-out ensureSession error = %v, want timeout", err)
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first session creator did not start")
	}

	id, err := service.ensureSession(time.Second)
	if err != nil {
		t.Fatalf("recovery ensureSession failed: %v", err)
	}
	if id != "recovered-session" {
		t.Fatalf("recovered session ID = %q, want recovered-session", id)
	}

	close(firstRelease)
	select {
	case id := <-closed:
		if id != "late-session" {
			t.Fatalf("late session cleanup ID = %q, want late-session", id)
		}
	case <-time.After(time.Second):
		t.Fatal("late session was not closed after timed-out flight")
	}
	if got := creates.Load(); got != 2 {
		t.Fatalf("session creator calls = %d, want 2", got)
	}
	service.sessionMu.Lock()
	defer service.sessionMu.Unlock()
	if service.sessionID != "recovered-session" {
		t.Fatalf("sessionID = %q, late creator overwrote recovered session", service.sessionID)
	}
}

func TestLauncherSessionConcurrentGetIsSingleFlight(t *testing.T) {
	var mu sync.Mutex
	creates := 0
	service := &LauncherService{
		createSessionFn: func() (*SessionInfo, error) {
			mu.Lock()
			creates++
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			return &SessionInfo{ID: "launcher-concurrent", Running: true}, nil
		},
		sessionExistsFn: func(id string) bool { return id == "launcher-concurrent" },
		sessionShutdown: make(chan struct{}),
	}

	const callers = 16
	ids := make(chan string, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := service.GetSessionID()
			ids <- id
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent GetSessionID failed: %v", err)
		}
	}
	for id := range ids {
		if id != "launcher-concurrent" {
			t.Fatalf("concurrent GetSessionID returned %q", id)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if creates != 1 {
		t.Fatalf("createSession called %d times, want 1", creates)
	}
}

func TestLauncherSessionFailureThenLazyRecovery(t *testing.T) {
	var mu sync.Mutex
	creates := 0
	service := &LauncherService{
		createSessionFn: func() (*SessionInfo, error) {
			mu.Lock()
			creates++
			attempt := creates
			mu.Unlock()
			if attempt == 1 {
				return nil, errors.New("shell unavailable")
			}
			return &SessionInfo{ID: "launcher-recovered", Running: true}, nil
		},
		sessionExistsFn: func(id string) bool { return id == "launcher-recovered" },
		sessionShutdown: make(chan struct{}),
	}

	if _, err := service.ensureSession(time.Second); err == nil {
		t.Fatal("eager session failure returned nil error")
	}
	if status := service.GetStatus(); !strings.Contains(status.Error, "launcher terminal unavailable") {
		t.Fatalf("status = %+v, want startup error", status)
	}
	id, err := service.GetSessionID()
	if err != nil {
		t.Fatalf("lazy recovery failed: %v", err)
	}
	if id != "launcher-recovered" {
		t.Fatalf("recovered ID = %q", id)
	}
	if status := service.GetStatus(); status.Error != "" {
		t.Fatalf("recovered status = %+v, want cleared session error", status)
	}
}

func TestLauncherSessionClosedIDIsReplacedAndClosed(t *testing.T) {
	active := map[string]bool{}
	var closed []string
	creates := 0
	service := &LauncherService{
		createSessionFn: func() (*SessionInfo, error) {
			creates++
			id := "launcher-1"
			if creates > 1 {
				id = "launcher-2"
			}
			active[id] = true
			return &SessionInfo{ID: id, Running: true}, nil
		},
		sessionExistsFn: func(id string) bool { return active[id] },
		closeSessionFn: func(id string) error {
			closed = append(closed, id)
			delete(active, id)
			return nil
		},
		sessionShutdown: make(chan struct{}),
	}

	first, err := service.GetSessionID()
	if err != nil {
		t.Fatalf("initial GetSessionID failed: %v", err)
	}
	delete(active, first)
	second, err := service.GetSessionID()
	if err != nil {
		t.Fatalf("replacement GetSessionID failed: %v", err)
	}
	if second == first || len(closed) != 1 || closed[0] != first {
		t.Fatalf("replacement = %q, closed = %v, want old ID closed once", second, closed)
	}
}

func TestLauncherSessionReusesIDDuringTerminalRestart(t *testing.T) {
	creates := 0
	service := &LauncherService{
		createSessionFn: func() (*SessionInfo, error) {
			creates++
			return &SessionInfo{ID: "launcher-restarting", Running: false}, nil
		},
		// TerminalService keeps the session record while its PTY is being
		// restarted, so the ID remains valid and Write can reuse it.
		sessionExistsFn: func(id string) bool { return id == "launcher-restarting" },
		sessionShutdown: make(chan struct{}),
	}

	first, err := service.GetSessionID()
	if err != nil {
		t.Fatalf("initial GetSessionID failed: %v", err)
	}
	second, err := service.GetSessionID()
	if err != nil {
		t.Fatalf("restart GetSessionID failed: %v", err)
	}
	if first != second || creates != 1 {
		t.Fatalf("restart IDs = %q, %q and creates = %d, want one reused ID", first, second, creates)
	}
}

func TestLauncherSessionShutdownClosesExactlyOneSession(t *testing.T) {
	closed := 0
	service := &LauncherService{
		createSessionFn: func() (*SessionInfo, error) {
			return &SessionInfo{ID: "launcher-shutdown", Running: true}, nil
		},
		sessionExistsFn: func(id string) bool { return id == "launcher-shutdown" },
		closeSessionFn: func(id string) error {
			if id != "launcher-shutdown" {
				t.Fatalf("closed unexpected session %q", id)
			}
			closed++
			return nil
		},
		sessionShutdown: make(chan struct{}),
	}
	if _, err := service.GetSessionID(); err != nil {
		t.Fatalf("GetSessionID failed: %v", err)
	}
	if err := service.ServiceShutdown(); err != nil {
		t.Fatalf("ServiceShutdown failed: %v", err)
	}
	if err := service.ServiceShutdown(); err != nil {
		t.Fatalf("second ServiceShutdown failed: %v", err)
	}
	if closed != 1 {
		t.Fatalf("closeSession called %d times, want exactly 1", closed)
	}
	if _, err := service.GetSessionID(); err == nil {
		t.Fatal("GetSessionID succeeded after shutdown")
	}
}
