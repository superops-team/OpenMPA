// OpenMPA's desktop entry point is pure assembly until MyGo calls WhenReady.
package main

import (
	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/artifact"
	"github.com/superops-team/OpenMPA/desktop/internal/channels"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/credentials"
	"github.com/superops-team/OpenMPA/desktop/internal/deployment"
	"github.com/superops-team/OpenMPA/desktop/internal/host"
	"github.com/superops-team/OpenMPA/desktop/internal/ipc"
	"github.com/superops-team/OpenMPA/desktop/internal/knowledge"
	"github.com/superops-team/OpenMPA/desktop/internal/nativeui"
	"github.com/superops-team/OpenMPA/desktop/internal/runtimeconfig"
	"github.com/superops-team/OpenMPA/desktop/internal/schedule"
	"github.com/superops-team/OpenMPA/desktop/internal/session"
	"github.com/superops-team/OpenMPA/desktop/internal/workspace"
	"log"
	"path/filepath"
)

func main() {
	mygo.App.SetName("OpenMPA")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	cloudClient := cloud.NewWithProfileStore(func() (string, error) {
		dir, e := mygo.App.Path(mygo.PathUserData)
		if e != nil {
			return "", e
		}
		return filepath.Join(dir, "credential-profiles.json"), nil
	})
	conn := connection.New()
	platform := ipc.NewPlatformWithPersistence(cloudClient, conn, credentials.New(), func() (string, error) {
		dir, e := mygo.App.Path(mygo.PathUserData)
		if e != nil {
			return "", e
		}
		return filepath.Join(dir, "saved-logins.json"), nil
	})
	shell := host.New()
	dataPath := func(name string) (string, error) {
		dir, e := mygo.App.Path(mygo.PathUserData)
		if e != nil {
			return "", e
		}
		return filepath.Join(dir, name), nil
	}
	documents := workspace.New(func() (string, error) { return dataPath("workspace.json") })
	pending := session.NewPendingStore(func() (string, error) { return dataPath("pending") })
	mygo.BindAs("Platform", platform)
	connectionService := &ipc.Connection{Client: conn, Platform: platform}
	channelService := channels.New(conn)
	knowledgeService := knowledge.New(conn)
	scheduleService := schedule.New(conn)
	deploymentService := deployment.NewWithLedger(cloud.DeploymentProvider{Client: cloudClient}, func() deployment.Target { return ipc.DeploymentTarget(platform) }, func() (string, error) { return dataPath("deployment-operations.json") })
	configService := runtimeconfig.New(cloudClient, func() deployment.Target { return ipc.DeploymentTarget(platform) }, func() (string, error) { return dataPath("runtime-config-operations") })
	mygo.BindAs("Connection", connectionService)
	sessions := session.New(conn, pending)
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
	mygo.App.WhenReady(func() {
		w, page, err := host.NewDesktopWindow(shell, mygo.WindowOptions{Title: "OpenMPA", URL: "/desktop.html", Width: 1280, Height: 850, TitleBarStyle: mygo.TitleBarHiddenInset, TrafficLightPosition: &mygo.Point{X: 14, Y: 15}})
		if err != nil {
			log.Fatal("OpenMPA desktop view could not start")
		}
		host.Attach(shell, w)
		nativeui.Attach(shell, w, nativeui.Services{Workspace: documents, Artifact: artifactService, Session: sessions, TaskIntentPath: func() (string, error) { return dataPath("native-task-intents.json") }, Platform: platform, Connection: connectionService, Channels: channelService, Knowledge: knowledgeService, Schedule: scheduleService, Deployment: deploymentService, RuntimeConfig: configService})
		host.RestrictPageNavigation(page)
		host.RestrictPagePermissions(page)
	})
	mygo.App.OnQuit(func() { conn.Disconnect(); workspace.Shutdown(documents) })
	if e := mygo.App.Run(); e != nil {
		log.Fatal("OpenMPA desktop could not start")
	}
}
