package worker

import (
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/models"
)

// ---------------------------------------------------------------------------
// CronRunner lifecycle tests
// ---------------------------------------------------------------------------

func cronTestDB(t *testing.T) (*Engine, *models.Site) {
	t.Helper()
	db := testDB(t)
	// Also migrate cron-related models.
	if err := db.AutoMigrate(
		&models.Site{},
		&models.CronSchedule{},
		&models.WorkerEnvVar{},
	); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}

	e := newTestEngine(t, db)

	v := 1
	site := models.Site{
		ID:            "cron-site",
		UserID:        "user1",
		SubdomainSlug: "cron-test",
		Name:          "Cron Test",
		HasWorker:     true,
		ActiveVersion: &v,
	}
	db.Create(&site)

	return e, &site
}

func TestCronRunner_StartAndShutdown(t *testing.T) {
	db := testDB(t)
	db.AutoMigrate(&models.Site{}, &models.CronSchedule{}, &models.WorkerEnvVar{})

	e := newTestEngine(t, db)
	cr := NewCronRunner(db, e)

	// Should start without error and shut down cleanly.
	cr.Shutdown()
}

func TestCronRunner_TickDispatchesMatchingSchedule(t *testing.T) {
	e, site := cronTestDB(t)

	// Compile a worker that logs on scheduled event.
	source := `export default {
  async scheduled(event, env, ctx) {
    console.log("cron executed: " + event.cron);
  },
  fetch(request) { return new Response("ok"); },
};`
	if _, err := e.CompileAndCache(site.ID, 1, source); err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Create a schedule matching "every minute".
	sched := models.CronSchedule{
		SiteID:  site.ID,
		Cron:    "* * * * *", // matches every minute
		Enabled: true,
	}
	e.db.Create(&sched)

	// Create and call tick manually.
	cr := &CronRunner{
		db:     e.db,
		engine: e,
		done:   make(chan struct{}),
	}
	cr.tick(time.Now())

	// Give dispatch goroutine time to complete.
	time.Sleep(500 * time.Millisecond)

	// Verify last_run_at was updated.
	var updated models.CronSchedule
	e.db.First(&updated, "id = ?", sched.ID)
	if updated.LastRunAt == nil {
		t.Error("last_run_at should be set after dispatch")
	}
}

func TestCronRunner_SkipsDisabledSchedule(t *testing.T) {
	e, site := cronTestDB(t)

	source := `export default {
  async scheduled(event, env, ctx) {
    console.log("should not run");
  },
  fetch(request) { return new Response("ok"); },
};`
	e.CompileAndCache(site.ID, 1, source)

	sched := models.CronSchedule{
		SiteID:  site.ID,
		Cron:    "* * * * *",
		Enabled: true,
	}
	e.db.Create(&sched)
	// GORM treats false as zero-value and applies default:true, so update after create.
	e.db.Model(&sched).Update("enabled", false)

	cr := &CronRunner{
		db:     e.db,
		engine: e,
		done:   make(chan struct{}),
	}
	cr.tick(time.Now())

	time.Sleep(200 * time.Millisecond)

	var updated models.CronSchedule
	e.db.First(&updated, "id = ?", sched.ID)
	if updated.LastRunAt != nil {
		t.Error("disabled schedule should not have last_run_at set")
	}
}

func TestCronRunner_SkipsNonMatchingTime(t *testing.T) {
	e, site := cronTestDB(t)

	source := `export default {
  async scheduled(event, env, ctx) {
    console.log("should not run");
  },
  fetch(request) { return new Response("ok"); },
};`
	e.CompileAndCache(site.ID, 1, source)

	// Schedule that only matches at midnight on Jan 1.
	sched := models.CronSchedule{
		SiteID:  site.ID,
		Cron:    "0 0 1 1 *",
		Enabled: true,
	}
	e.db.Create(&sched)

	// Tick at a time that doesn't match (June 15, 3pm).
	fakeTime := time.Date(2025, 6, 15, 15, 30, 0, 0, time.UTC)
	cr := &CronRunner{
		db:     e.db,
		engine: e,
		done:   make(chan struct{}),
	}
	cr.tick(fakeTime)

	time.Sleep(200 * time.Millisecond)

	var updated models.CronSchedule
	e.db.First(&updated, "id = ?", sched.ID)
	if updated.LastRunAt != nil {
		t.Error("non-matching schedule should not have last_run_at set")
	}
}

func TestCronRunner_SkipsSiteWithNoWorker(t *testing.T) {
	db := testDB(t)
	db.AutoMigrate(&models.Site{}, &models.CronSchedule{}, &models.WorkerEnvVar{}, &models.KVNamespace{})

	e := newTestEngine(t, db)

	v := 1
	site := models.Site{
		ID:            "no-worker-site",
		UserID:        "user1",
		SubdomainSlug: "no-worker",
		Name:          "No Worker",
		HasWorker:     false, // no worker
		ActiveVersion: &v,
	}
	db.Create(&site)

	sched := models.CronSchedule{
		SiteID:  site.ID,
		Cron:    "* * * * *",
		Enabled: true,
	}
	db.Create(&sched)

	cr := &CronRunner{
		db:     db,
		engine: e,
		done:   make(chan struct{}),
	}
	cr.tick(time.Now())

	time.Sleep(200 * time.Millisecond)

	var updated models.CronSchedule
	db.First(&updated, "id = ?", sched.ID)
	if updated.LastRunAt != nil {
		t.Error("site without worker should not be dispatched")
	}
}

func TestCronRunner_StoresLogs(t *testing.T) {
	e, site := cronTestDB(t)

	source := `export default {
  async scheduled(event, env, ctx) {
    console.log("cron log message");
    console.warn("cron warning");
  },
  fetch(request) { return new Response("ok"); },
};`
	e.CompileAndCache(site.ID, 1, source)

	sched := models.CronSchedule{
		SiteID:  site.ID,
		Cron:    "* * * * *",
		Enabled: true,
	}
	e.db.Create(&sched)

	cr := &CronRunner{
		db:     e.db,
		engine: e,
		done:   make(chan struct{}),
	}
	cr.tick(time.Now())

	time.Sleep(500 * time.Millisecond)

	var logs []models.WorkerLog
	e.db.Where("site_id = ?", site.ID).Find(&logs)
	if len(logs) < 2 {
		t.Errorf("expected at least 2 logs, got %d", len(logs))
	}
}
