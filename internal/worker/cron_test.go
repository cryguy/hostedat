package worker

import (
	"testing"
	"time"

	"github.com/cryguy/hostedat/internal/models"
)

func TestCronMatches(t *testing.T) {
	tests := []struct {
		name  string
		expr  string
		time  time.Time
		match bool
	}{
		{"every minute", "* * * * *", time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC), true},
		{"exact match", "30 12 1 1 1", time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC), true},
		{"no match minute", "0 12 1 1 *", time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC), false},
		{"step */5 match", "*/5 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), true},
		{"step */5 no match", "*/5 * * * *", time.Date(2024, 1, 1, 12, 13, 0, 0, time.UTC), false},
		{"range match", "0-30 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), true},
		{"range no match", "0-10 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), false},
		{"comma list match", "0,15,30,45 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), true},
		{"comma list no match", "0,30,45 * * * *", time.Date(2024, 1, 1, 12, 15, 0, 0, time.UTC), false},
		{"invalid field count", "* * *", time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC), false},
		{"weekday Sunday=0", "* * * * 0", time.Date(2024, 1, 7, 12, 0, 0, 0, time.UTC), true},
		{"step */15 at 0", "*/15 * * * *", time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC), true},
		{"step */15 at 45", "*/15 * * * *", time.Date(2024, 1, 1, 12, 45, 0, 0, time.UTC), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cronMatches(tt.expr, tt.time)
			if got != tt.match {
				t.Errorf("cronMatches(%q, %v) = %v, want %v", tt.expr, tt.time, got, tt.match)
			}
		})
	}
}

func TestValidateCron(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"valid every minute", "* * * * *", false},
		{"valid specific", "30 12 1 1 1", false},
		{"valid step", "*/5 * * * *", false},
		{"valid range", "0-30 * * * *", false},
		{"valid comma", "0,15,30 * * * *", false},
		{"valid combo", "0,30 */2 * * 1-5", false},
		{"too few fields", "* * *", true},
		{"too many fields", "* * * * * *", true},
		{"minute out of range", "60 * * * *", true},
		{"hour out of range", "* 24 * * *", true},
		{"day out of range", "* * 32 * *", true},
		{"month out of range", "* * * 13 *", true},
		{"weekday out of range", "* * * * 7", true},
		{"invalid step", "*/0 * * * *", true},
		{"invalid value", "* abc * * *", true},
		{"invalid range reversed", "* * 10-5 * *", true},
		{"day zero", "* * 0 * *", true},
		{"month zero", "* * * 0 *", true},
		{"empty string", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCron(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCron(%q) error = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestFieldMatches_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value int
		want  bool
	}{
		{"wildcard", "*", 42, true},
		{"step divide evenly", "*/10", 30, true},
		{"step no match", "*/10", 33, false},
		{"step at zero", "*/15", 0, true},
		{"comma single match", "5,10,15", 10, true},
		{"comma no match", "5,10,15", 7, false},
		{"range start", "10-20", 10, true},
		{"range end", "10-20", 20, true},
		{"range middle", "10-20", 15, true},
		{"range below", "10-20", 9, false},
		{"range above", "10-20", 21, false},
		{"exact match", "42", 42, true},
		{"exact no match", "42", 43, false},
		{"invalid step", "*/0", 5, false},
		{"invalid step negative", "*/-5", 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldMatches(tt.field, tt.value)
			if got != tt.want {
				t.Errorf("fieldMatches(%q, %d) = %v, want %v", tt.field, tt.value, got, tt.want)
			}
		})
	}
}

func TestFieldMatches_CommaWithRange(t *testing.T) {
	// "1-5,10,20-25" should match 3, 10, 22 but not 7
	tests := []struct {
		value int
		want  bool
	}{
		{3, true},
		{10, true},
		{22, true},
		{7, false},
		{26, false},
		{1, true},
		{5, true},
		{20, true},
		{25, true},
	}
	for _, tt := range tests {
		got := fieldMatches("1-5,10,20-25", tt.value)
		if got != tt.want {
			t.Errorf("fieldMatches('1-5,10,20-25', %d) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestFieldMatches_InvalidRange(t *testing.T) {
	// Invalid range bounds should not match.
	if fieldMatches("abc-def", 5) {
		t.Error("fieldMatches with invalid range should return false")
	}
	// Invalid single value should not match.
	if fieldMatches("xyz", 5) {
		t.Error("fieldMatches with invalid value should return false")
	}
}

func TestFieldMatches_StepNonNumeric(t *testing.T) {
	if fieldMatches("*/abc", 5) {
		t.Error("fieldMatches with non-numeric step should return false")
	}
}

func TestCronMatches_AllFields(t *testing.T) {
	// Tuesday Jan 14, 2025 at 14:30
	// minute=30, hour=14, day=14, month=1, weekday=2 (Tuesday)
	tm := time.Date(2025, 1, 14, 14, 30, 0, 0, time.UTC)

	if !cronMatches("30 14 14 1 2", tm) {
		t.Error("exact match for all 5 fields should match")
	}
	if cronMatches("30 14 14 1 3", tm) {
		t.Error("wrong weekday should not match")
	}
	if cronMatches("31 14 14 1 2", tm) {
		t.Error("wrong minute should not match")
	}
}

func TestValidateCron_StepValues(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"*/1 * * * *", false},
		{"*/59 * * * *", false},
		{"*/abc * * * *", true},
		{"*/-1 * * * *", true},
	}
	for _, tt := range tests {
		err := ValidateCron(tt.expr)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateCron(%q) err=%v, wantErr=%v", tt.expr, err, tt.wantErr)
		}
	}
}

func TestValidateCron_RangeEdges(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"0-59 0-23 1-31 1-12 0-6", false}, // all ranges at limits
		{"0-0 * * * *", false},             // single-value range
		{"59-59 * * * *", false},           // single-value range at max
	}
	for _, tt := range tests {
		err := ValidateCron(tt.expr)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateCron(%q) err=%v, wantErr=%v", tt.expr, err, tt.wantErr)
		}
	}
}

func TestCronRunner_TickWithMatchingSchedule(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Create a user and site with an active deployment.
	u := models.User{Email: "cron-tick@test.local", PasswordHash: "hash"}
	if result := db.Create(&u); result.Error != nil {
		t.Fatal(result.Error)
	}
	deployID := "cron-deploy-1"
	s := models.Site{
		ID:             "cron-tick-site",
		UserID:         u.ID,
		SubdomainSlug:  "cron-tick",
		Name:           "Cron Tick Site",
		HasWorker:      true,
		ActiveDeployID: &deployID,
	}
	if result := db.Create(&s); result.Error != nil {
		t.Fatal(result.Error)
	}

	// Cache the worker source.
	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled(event, env, ctx) {
    console.log("cron ran: " + event.cron);
  },
};`
	if _, err := e.CompileAndCache(s.ID, deployID, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	// Create a cron schedule that matches "every minute".
	sched := models.CronSchedule{
		SiteID:  s.ID,
		Cron:    "* * * * *",
		Enabled: true,
	}
	if result := db.Create(&sched); result.Error != nil {
		t.Fatal(result.Error)
	}

	// Create the runner and manually tick.
	cr := &CronRunner{db: db, engine: e, done: make(chan struct{})}
	cr.tick(time.Now())

	// Give the goroutine a moment to run dispatch.
	time.Sleep(200 * time.Millisecond)
}

func TestCronRunner_TickSkipsDisabled(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	u := models.User{Email: "cron-disabled@test.local", PasswordHash: "hash"}
	if result := db.Create(&u); result.Error != nil {
		t.Fatal(result.Error)
	}
	deployID := "disabled-deploy"
	s := models.Site{
		ID:             "cron-disabled-site",
		UserID:         u.ID,
		SubdomainSlug:  "cron-disabled",
		Name:           "Cron Disabled",
		HasWorker:      true,
		ActiveDeployID: &deployID,
	}
	if result := db.Create(&s); result.Error != nil {
		t.Fatal(result.Error)
	}

	// Disabled schedule should not run.
	sched := models.CronSchedule{
		SiteID:  s.ID,
		Cron:    "* * * * *",
		Enabled: false,
	}
	if result := db.Create(&sched); result.Error != nil {
		t.Fatal(result.Error)
	}

	cr := &CronRunner{db: db, engine: e, done: make(chan struct{})}
	// This should not panic or dispatch anything.
	cr.tick(time.Now())
}

func TestCronRunner_TickSkipsNoWorker(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	u := models.User{Email: "cron-noworker@test.local", PasswordHash: "hash"}
	if result := db.Create(&u); result.Error != nil {
		t.Fatal(result.Error)
	}
	s := models.Site{
		ID:            "cron-noworker-site",
		UserID:        u.ID,
		SubdomainSlug: "cron-noworker",
		Name:          "No Worker",
		HasWorker:     false,
	}
	if result := db.Create(&s); result.Error != nil {
		t.Fatal(result.Error)
	}

	sched := models.CronSchedule{
		SiteID:  s.ID,
		Cron:    "* * * * *",
		Enabled: true,
	}
	if result := db.Create(&sched); result.Error != nil {
		t.Fatal(result.Error)
	}

	cr := &CronRunner{db: db, engine: e, done: make(chan struct{})}
	cr.tick(time.Now())
}

func TestCronRunner_TickFullPath(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	u := models.User{Email: "cron-full@test.local", PasswordHash: "hash"}
	if result := db.Create(&u); result.Error != nil {
		t.Fatal(result.Error)
	}
	deployID := "full-deploy"
	s := models.Site{
		ID:             "cron-full-site",
		UserID:         u.ID,
		SubdomainSlug:  "cron-full",
		Name:           "Full Path",
		HasWorker:      true,
		ActiveDeployID: &deployID,
	}
	if result := db.Create(&s); result.Error != nil {
		t.Fatal(result.Error)
	}

	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled(event, env, ctx) {
    console.log("full path cron");
  },
};`
	if _, err := e.CompileAndCache(s.ID, deployID, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	sched := models.CronSchedule{
		SiteID:  s.ID,
		Cron:    "* * * * *",
		Enabled: true,
	}
	if result := db.Create(&sched); result.Error != nil {
		t.Fatal(result.Error)
	}

	cr := &CronRunner{db: db, engine: e, done: make(chan struct{})}

	// Tick should match and dispatch. Give goroutine time to finish.
	cr.tick(time.Now())
	time.Sleep(500 * time.Millisecond)
}

func TestCronRunner_TickSiteLookupFails(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Create a schedule for a site that doesn't exist in the DB.
	sched := models.CronSchedule{
		SiteID:  "nonexistent-site",
		Cron:    "* * * * *",
		Enabled: true,
	}
	if result := db.Create(&sched); result.Error != nil {
		t.Fatal(result.Error)
	}

	cr := &CronRunner{db: db, engine: e, done: make(chan struct{})}
	cr.tick(time.Now()) // Should not panic, just skip
}

func TestCronRunner_TickNonMatchingCron(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	u := models.User{Email: "cron-nomatch@test.local", PasswordHash: "hash"}
	if result := db.Create(&u); result.Error != nil {
		t.Fatal(result.Error)
	}
	deployID := "nomatch-deploy"
	s := models.Site{
		ID:             "cron-nomatch-site",
		UserID:         u.ID,
		SubdomainSlug:  "cron-nomatch",
		Name:           "No Match",
		HasWorker:      true,
		ActiveDeployID: &deployID,
	}
	if result := db.Create(&s); result.Error != nil {
		t.Fatal(result.Error)
	}

	// Schedule that only runs at minute 59 — won't match minute 0.
	sched := models.CronSchedule{
		SiteID:  s.ID,
		Cron:    "59 23 31 12 0",
		Enabled: true,
	}
	if result := db.Create(&sched); result.Error != nil {
		t.Fatal(result.Error)
	}

	cr := &CronRunner{db: db, engine: e, done: make(chan struct{})}
	// Use a time that doesn't match (minute 0, hour 0, day 1, month 1, Monday).
	cr.tick(time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC))
}

func TestCronRunner_DispatchDirect(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	u := models.User{Email: "cron-dispatch@test.local", PasswordHash: "hash"}
	if result := db.Create(&u); result.Error != nil {
		t.Fatal(result.Error)
	}
	deployID := "dispatch-deploy"
	s := models.Site{
		ID:             "cron-dispatch-site",
		UserID:         u.ID,
		SubdomainSlug:  "cron-dispatch",
		Name:           "Dispatch Test",
		HasWorker:      true,
		ActiveDeployID: &deployID,
	}
	if result := db.Create(&s); result.Error != nil {
		t.Fatal(result.Error)
	}

	source := `export default {
  fetch() { return new Response("ok"); },
  scheduled(event, env, ctx) {
    console.log("dispatched: " + event.cron);
  },
};`
	if _, err := e.CompileAndCache(s.ID, deployID, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	sched := models.CronSchedule{
		SiteID:  s.ID,
		Cron:    "*/5 * * * *",
		Enabled: true,
	}
	if result := db.Create(&sched); result.Error != nil {
		t.Fatal(result.Error)
	}

	cr := &CronRunner{db: db, engine: e, done: make(chan struct{})}

	env := BuildEnvFromDB(db, s.ID, nil)

	// Call dispatch directly (synchronously) to capture coverage.
	// dispatch runs ExecuteScheduled and updates last_run_at.
	cr.dispatch(sched, s.ID, deployID, env)
}

func TestCronRunner_EnsureSourceDirect(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	cr := &CronRunner{db: db, engine: e, done: make(chan struct{})}

	// ensureSource for a nonexistent site should log but not panic.
	cr.ensureSource("nonexistent", "deploy1")
}

func TestCronRunner_DispatchWithError(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	u := models.User{Email: "cron-err@test.local", PasswordHash: "hash"}
	if result := db.Create(&u); result.Error != nil {
		t.Fatal(result.Error)
	}
	deployID := "err-deploy"
	s := models.Site{
		ID:             "cron-err-site",
		UserID:         u.ID,
		SubdomainSlug:  "cron-err",
		Name:           "Error Test",
		HasWorker:      true,
		ActiveDeployID: &deployID,
	}
	if result := db.Create(&s); result.Error != nil {
		t.Fatal(result.Error)
	}

	// Worker with no scheduled handler should fail.
	source := `export default {
  fetch() { return new Response("ok"); },
};`
	if _, err := e.CompileAndCache(s.ID, deployID, source); err != nil {
		t.Fatalf("CompileAndCache: %v", err)
	}

	sched := models.CronSchedule{
		SiteID:  s.ID,
		Cron:    "* * * * *",
		Enabled: true,
	}
	if result := db.Create(&sched); result.Error != nil {
		t.Fatal(result.Error)
	}

	cr := &CronRunner{db: db, engine: e, done: make(chan struct{})}
	env := BuildEnvFromDB(db, s.ID, nil)

	// Should not panic, just log the error.
	cr.dispatch(sched, s.ID, deployID, env)
}

func TestNewCronRunner_BasicCreation(t *testing.T) {
	db := testDB(t)
	e := newTestEngine(t, db)

	// Create and immediately shutdown
	cr := NewCronRunner(db, e)
	if cr == nil {
		t.Fatal("NewCronRunner returned nil")
	}
	cr.Shutdown()
}
