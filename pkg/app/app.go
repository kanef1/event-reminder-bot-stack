package app

import (
	"context"
	"errors"
	"time"

	"event-reminder-bot/pkg/botService"
	"event-reminder-bot/pkg/db"
	botManager "event-reminder-bot/pkg/event-reminder-bot"
	"event-reminder-bot/pkg/reminder"

	"github.com/go-pg/pg/v10"
	"github.com/go-telegram/bot"
	monitor "github.com/hypnoglow/go-pg-monitor"
	"github.com/labstack/echo/v4"
	"github.com/vmkteam/appkit"
	"github.com/vmkteam/embedlog"
)

type Config struct {
	Database *pg.Options
	Server   struct {
		Host      string
		Port      int
		IsDevel   bool
		EnableVFS bool
	}
	Sentry struct {
		Environment string
		DSN         string
	}
	Bot struct {
		Token string
	}
}

type App struct {
	embedlog.Logger
	appName string
	cfg     Config
	db      db.DB
	dbc     *pg.DB
	mon     *monitor.Monitor
	echo    *echo.Echo

	b          *bot.Bot
	bm         *botManager.BotManager
	rm         *reminder.ReminderManager
	bs         *botService.BotService
	eventsRepo db.EventsRepo
}

func New(appName string, sl embedlog.Logger, cfg Config, database db.DB, dbc *pg.DB) *App {
	a := &App{
		appName: appName,
		cfg:     cfg,
		db:      database,
		dbc:     dbc,
		echo:    appkit.NewEcho(),
		Logger:  sl,
	}

	a.eventsRepo = db.NewEventsRepo(a.dbc)

	if cfg.Bot.Token != "" {
		a.Errorf("Токен бота не указан, бот не будет запущен")
		return a
	}

	b, err := bot.New(cfg.Bot.Token)
	if err != nil {
		a.Errorf("Ошибка инициализации бота: %v", err)
		return a
	}

	a.b = b
	a.bm = botManager.NewBotManager(a.b, a.eventsRepo, sl)
	a.rm = reminder.NewReminderManager(a.bm, a.eventsRepo, sl)
	a.bs = botService.NewBotService(b, a.bm, a.rm)

	return a
}

func (a *App) Run(ctx context.Context) error {
	a.registerMetrics()
	a.registerHandlers()
	a.registerDebugHandlers()
	a.registerMetadata()
	a.registerCron(ctx)

	if a.b == nil {
		return errors.New("бот не запущен (токен не указан)")
	}

	a.bs.RegisterHandlers()

	go a.b.Start(ctx)
	a.Printf("Бот запущен")

	return a.runHTTPServer(ctx, a.cfg.Server.Host, a.cfg.Server.Port)
}

func (a *App) Shutdown(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if a.mon != nil {
		a.mon.Close()
	}

	if a.dbc != nil {
		err := a.dbc.Close()
		if err != nil {
			return err
		}
	}

	return a.echo.Shutdown(ctx)
}

// registerMetadata is a function that registers meta info from service. Must be updated.
func (a *App) registerMetadata() {
	opts := appkit.MetadataOpts{
		HasPublicAPI:  true,
		HasPrivateAPI: true,
		DBs: []appkit.DBMetadata{
			appkit.NewDBMetadata(a.cfg.Database.Database, a.cfg.Database.PoolSize, false),
		},
		Services: []appkit.ServiceMetadata{
			// NewServiceMetadata("srv", MetadataServiceTypeAsync),
		},
	}

	md := appkit.NewMetadataManager(opts)
	md.RegisterMetrics()

	a.echo.GET("/debug/metadata", md.Handler)
}
