// OpenMPA's desktop entry point assembles local services before MyGo calls WhenReady.
package main

import (
	"context"
	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/artifact"
	"github.com/superops-team/OpenMPA/desktop/internal/channels"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/ipc"
	"github.com/superops-team/OpenMPA/desktop/internal/knowledge"
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
	"github.com/superops-team/OpenMPA/desktop/internal/runtimeconfig"
	"github.com/superops-team/OpenMPA/desktop/internal/runtimeterminal"
	"github.com/superops-team/OpenMPA/desktop/internal/schedule"
	"github.com/superops-team/OpenMPA/desktop/internal/session"
	"github.com/superops-team/OpenMPA/desktop/internal/trace"
	"github.com/superops-team/OpenMPA/desktop/internal/usage"
	"github.com/superops-team/OpenMPA/desktop/internal/vault"
	"github.com/superops-team/OpenMPA/desktop/internal/workspace"
	"log"
	"os"
	"path/filepath"
	"sync"
)

func main() {
	mygo.App.SetName("OpenMPA")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	generateData := ""
	if os.Getenv("MYGO_GENERATE") != "" {
		var err error
		generateData, err = os.MkdirTemp("", "openmpa-generate-")
		if err != nil {
			log.Fatal("OpenMPA binding generation could not start")
		}
		defer os.RemoveAll(generateData)
		mygo.App.SetPath(mygo.PathUserData, generateData)
	}
	userData, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Fatal("OpenMPA local data directory could not start")
	}
	settingsDB, err := configstore.Open(filepath.Join(userData, "openmpa.db"))
	if err != nil {
		log.Fatal("OpenMPA local settings could not start")
	}
	mygo.App.SetWindowStateStore(settingsDB)
	cloudClient := cloud.New()
	conn := connection.New()
	platform := ipc.NewPlatformWithStore(cloudClient, conn, vault.New(settingsDB), settingsDB)
	shell := host.New()
	dataPath := func(name string) (string, error) {
		return filepath.Join(userData, name), nil
	}
	languageService := localization.New(settingsDB)
	mygo.App.OnLocaleChanged(localization.SetCurrentLocale)
	documents := workspace.New(func() (string, error) { return dataPath("workspace.json") })
	host.SetCanClose(shell, documents.SafeClose)
	pending := session.NewPendingStore(func() (string, error) { return dataPath("pending") })
	mygo.BindAs("Platform", platform)
	connectionService := &ipc.Connection{Client: conn, Platform: platform}
	channelService := channels.New(conn)
	knowledgeService := knowledge.New(conn)
	scheduleService := schedule.New(conn)
	deploymentService := deployment.NewWithLedger(cloud.DeploymentProvider{Client: cloudClient}, func() deployment.Target { return ipc.DeploymentTarget(platform) }, func() (string, error) { return dataPath("deployment-operations.json") })
	configService := runtimeconfig.New(cloudClient, func() deployment.Target { return ipc.DeploymentTarget(platform) }, func() (string, error) { return dataPath("runtime-config-operations") })
	mygo.BindAs("Connection", connectionService)
	mygo.BindAs("Usage", usage.NewBridge(usage.New(conn, cloudClient, func() deployment.Target { return ipc.DeploymentTarget(platform) })))
	sessions := session.NewWithCache(conn, pending, settingsDB, cloudClient)

	mygo.BindAs("Session", sessions)
	mygo.BindAs("Channels", channelService)
	artifactService := artifact.New(conn)
	mygo.BindAs("Artifact", artifactService)
	mygo.BindAs("Knowledge", knowledgeService)
	mygo.BindAs("Workspace", documents)
	mygo.BindAs("Host", shell)
	mygo.BindAs("Schedule", scheduleService)
	mygo.BindAs("Deployment", deploymentService)
	mygo.BindAs("RuntimeConfig", configService)
	traceService := trace.New(conn)
	traceCache, err := trace.NewCache(filepath.Join(userData, "trace-cache"))
	if err != nil {
		log.Fatal("OpenMPA trace cache could not start")
	}
	traceSweepCtx, stopTraceSweep := context.WithCancel(context.Background())
	traceCache.StartSweeper(traceSweepCtx)
	traceService = trace.NewWithCache(conn, traceCache)
	session.SetDeleteHook(sessions, func(sessionID string) { _ = traceCache.DeleteSession(sessionID) })
	var archiveWarning sync.Once
	session.SetEventHook(sessions, func(event dto.ClientEvent) {
		if err := traceCache.AppendEvent(event); err != nil {
			archiveWarning.Do(func() { log.Print("OpenMPA local execution archive could not be saved") })
		}
	})
	mygo.BindAs("Trace", traceService)
	terminalService := runtimeterminal.New(cloudClient, conn, func() dto.RuntimeRequest { return ipc.Target(platform) })
	mygo.BindAs("Terminal", runtimeterminal.NewIPC(terminalService))
	mygo.BindAs("Localization", languageService)
	mygo.App.WhenReady(func() {
		_ = localization.Load(languageService)
		w, page, err := host.NewDesktopWindow(shell, mygo.WindowOptions{Title: "OpenMPA", URL: "/desktop.html", Width: 1280, Height: 850, StateKey: "main", TitleBarStyle: mygo.TitleBarHiddenInset, TrafficLightPosition: &mygo.Point{X: 14, Y: 15}})
		if err != nil {
			log.Fatal("OpenMPA desktop view could not start")
		}
		host.Attach(shell, w)
		host.RestrictPageNavigation(page)
		host.RestrictPagePermissions(page)
	})
	mygo.App.OnQuit(func() {
		stopTraceSweep()
		terminalService.CloseAll()
		conn.Disconnect()
		workspace.Shutdown(documents)
		settingsDB.Close()
	})
	if e := mygo.App.Run(); e != nil {
		log.Fatal("OpenMPA desktop could not start")
	}
}
