package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/tray.png
var trayIcon []byte

func main() {
	appService, err := NewAppService()
	if err != nil {
		log.Fatal(err)
	}

	dockerService, err := NewDockerService()
	if err != nil {
		log.Fatal(err)
	}

	settings := appService.Snapshot()

	if settings.DockerEndpoint != "" {
		dockerService.SetConfiguredDockerEndpoint(
			settings.DockerEndpoint,
		)
	} else {
		settings.DockerEndpoint =
			dockerService.ConfiguredDockerEndpoint()

		if settings.DockerEndpoint != "" {
			if err := appService.UpdateSettings(
				settings,
			); err != nil {
				log.Printf(
					"could not persist Docker endpoint: %v",
					err,
				)
			}
		}
	}

	provider, err := dockerService.SelectEngineBackend(
		settings.EngineBackend,
		settings.WSLDistro,
		settings.DockerEndpoint,
	)
	if err != nil {
		log.Printf("could not restore engine backend %q: %v", settings.EngineBackend, err)
	} else if settings.RuntimeProvider != provider {
		settings.RuntimeProvider = provider
		if err := appService.UpdateSettings(settings); err != nil {
			log.Printf("could not persist active runtime %q: %v", provider, err)
		}
	}

	startHidden := settings.StartHidden || hasArg("--hidden")

	app := application.New(application.Options{
		Name:        "DevStack",
		Description: "Fast cross-platform container development environment",
		Services: []application.Service{
			application.NewService(dockerService),
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:           "main",
		Title:          "DevStack",
		Width:          1320,
		Height:         820,
		MinWidth:       980,
		MinHeight:      650,
		URL:            "/",
		EnableFileDrop: true,
		Hidden:         startHidden,
	})

	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		if len(files) > 0 {
			app.Event.Emit("devstack:files-dropped", files)
		}
	})

	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if appService.Snapshot().CloseToTray {
			window.Hide()
			event.Cancel()
		}
	})

	trayController := NewTrayController(
		app,
		window,
		dockerService,
		appService,
		trayIcon,
	)
	trayController.Start()

	go dockerService.AutoRecoverEngine(
		settings.EngineBackend,
		settings.WSLDistro,
		settings.AutoReconnectEngine,
		settings.StartEngineOnLaunch,
	)

	if err := app.Run(); err != nil {
		trayController.Close()
		log.Fatal(err)
	}

	trayController.Close()
}

func hasArg(target string) bool {
	for _, arg := range os.Args[1:] {
		if arg == target {
			return true
		}
	}
	return false
}
