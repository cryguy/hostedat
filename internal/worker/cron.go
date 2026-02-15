package worker

import (
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cryguy/hostedat/internal/models"
	"gorm.io/gorm"
)

// CronRunner ticks every minute, evaluates schedules, and dispatches
// worker scheduled() handlers for matching cron entries.
type CronRunner struct {
	db     *gorm.DB
	engine *Engine
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewCronRunner creates and starts a CronRunner that evaluates schedules
// every minute. Call Shutdown() to stop it.
func NewCronRunner(db *gorm.DB, engine *Engine) *CronRunner {
	cr := &CronRunner{
		db:     db,
		engine: engine,
		done:   make(chan struct{}),
	}
	cr.wg.Add(1)
	go cr.run()
	return cr
}

// Shutdown stops the cron runner and waits for it to finish.
func (cr *CronRunner) Shutdown() {
	close(cr.done)
	cr.wg.Wait()
}

func (cr *CronRunner) run() {
	defer cr.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-cr.done:
			return
		case now := <-ticker.C:
			cr.tick(now)
		}
	}
}

func (cr *CronRunner) tick(now time.Time) {
	var schedules []models.CronSchedule
	cr.db.Where("enabled = ?", true).Find(&schedules)

	for _, sched := range schedules {
		if !cronMatches(sched.Cron, now) {
			continue
		}

		// Look up site to get active version.
		var site models.Site
		if err := cr.db.First(&site, "id = ?", sched.SiteID).Error; err != nil {
			continue
		}
		if site.ActiveVersion == nil || !site.HasWorker {
			continue
		}

		version := *site.ActiveVersion

		// Load bytecode if not cached (server restart scenario).
		cr.ensureBytecode(site.ID, version)

		// Build env.
		env := cr.buildEnv(&site, version)

		// Dispatch in goroutine.
		go cr.dispatch(sched, site.ID, version, env)
	}
}

func (cr *CronRunner) ensureBytecode(siteID string, version int) {
	if err := cr.engine.EnsureBytecode(siteID, version); err != nil {
		log.Printf("cron: failed to ensure bytecode for site %s v%d: %v", siteID, version, err)
	}
}

func (cr *CronRunner) buildEnv(site *models.Site, version int) *Env {
	env := &Env{
		Vars:       make(map[string]string),
		Secrets:    make(map[string]string),
		KVBindings: make(map[string]string),
	}

	var envVars []models.WorkerEnvVar
	cr.db.Where("site_id = ?", site.ID).Find(&envVars)
	for _, ev := range envVars {
		if ev.Secret {
			env.Secrets[ev.Name] = ev.Value
		} else {
			env.Vars[ev.Name] = ev.Value
		}
	}

	var kvNamespaces []models.KVNamespace
	cr.db.Where("site_id = ?", site.ID).Find(&kvNamespaces)
	for _, ns := range kvNamespaces {
		env.KVBindings[ns.Name] = ns.ID
	}

	return env
}

func (cr *CronRunner) dispatch(sched models.CronSchedule, siteID string, version int, env *Env) {
	result := cr.engine.ExecuteScheduled(siteID, version, env, sched.Cron)

	// Update last_run_at.
	cr.db.Model(&sched).Update("last_run_at", time.Now())

	// Store logs.
	if len(result.Logs) > 0 {
		for _, l := range result.Logs {
			cr.db.Create(&models.WorkerLog{
				SiteID:    siteID,
				Level:     l.Level,
				Message:   l.Message,
				CreatedAt: l.Time,
			})
		}
	}

	if result.Error != nil {
		log.Printf("cron error for site %s schedule %s: %v", siteID, sched.Cron, result.Error)
	}
}

// cronMatches checks if the given cron expression matches the current time.
// Supports standard 5-field cron: minute hour day-of-month month day-of-week.
// Fields support: *, exact numbers, */N step values.
func cronMatches(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}

	values := []int{t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())}

	for i, field := range fields {
		if !fieldMatches(field, values[i]) {
			return false
		}
	}
	return true
}

func fieldMatches(field string, value int) bool {
	if field == "*" {
		return true
	}

	// Step: */N
	if strings.HasPrefix(field, "*/") {
		step, err := strconv.Atoi(field[2:])
		if err != nil || step <= 0 {
			return false
		}
		return value%step == 0
	}

	// Comma-separated values
	for _, part := range strings.Split(field, ",") {
		// Range: N-M
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			low, err1 := strconv.Atoi(bounds[0])
			high, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil {
				continue
			}
			if value >= low && value <= high {
				return true
			}
			continue
		}

		// Exact value
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		if n == value {
			return true
		}
	}

	return false
}
