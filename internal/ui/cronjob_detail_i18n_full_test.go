package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/chenpu/k8s-monitor/internal/model"
)

// renderCronJobDetail and its section renderers bilingual lockdown: the
// English output must stay byte-identical to the previously hardcoded strings
// (including leading indentation), and the Chinese output must switch.
// Reused keys keep their family values: detail.namespace / detail.age /
// detail.status labels, the columns.* table headers and the
// workloads.jobs.status_* job status values; the suspend True/False boolean,
// the cron expression and job/pod names are k8s values and stay untranslated.
// forcedAsciiProfile keeps the goldens plain text.

func cronJobFixtures(created, last time.Time) (*model.CronJobData, *model.CronJobData) {
	minimal := &model.CronJobData{Name: "cj-x", Namespace: "default", CreationTimestamp: created}
	full := &model.CronJobData{
		Name: "cj-x", Namespace: "default", CreationTimestamp: created,
		Schedule: "*/5 * * * *", LastScheduleTime: last,
		Labels: map[string]string{"env": "prod", "team": "api"},
	}
	return minimal, full
}

func TestRenderCronJobBasicInfoEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	// Golden pins "5m", which holds while the fixture-to-render delay stays
	// under 60s — the -5m offset sits exactly at the lower edge of the
	// [5m,6m) window, so any real execution delay only moves deeper into it.
	minimal, full := cronJobFixtures(time.Now().Add(-5*time.Minute), time.Now())

	wantMinimal := " ⏰ CronJob: cj-x \n\n" +
		"  Namespace: default\n" +
		"  Age: 5m"
	if got := en.renderCronJobBasicInfo(minimal); got != wantMinimal {
		t.Errorf("en minimal basic info:\n got %q\nwant %q", got, wantMinimal)
	}

	wantFull := " ⏰ CronJob: cj-x \n\n" +
		"  Namespace: default\n" +
		"  Age: 5m"
	if got := en.renderCronJobBasicInfo(full); got != wantFull {
		t.Errorf("en full basic info:\n got %q\nwant %q", got, wantFull)
	}
}

func TestRenderCronJobBasicInfoZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	minimal, _ := cronJobFixtures(time.Now().Add(-5*time.Minute), time.Now())

	got := zh.renderCronJobBasicInfo(minimal)
	for _, noun := range []string{"⏰ 定时任务：cj-x", "命名空间", "年龄"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh basic info %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"CronJob:", "Namespace", "Age:", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh basic info %q contains %q", got, bad)
		}
	}
}

func TestRenderCronJobScheduleEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	active := &model.CronJobData{Name: "cj-x", Namespace: "default", Schedule: "*/5 * * * *"}
	want := "Schedule\n\n" +
		"  Schedule: */5 * * * *\n" +
		"  Suspend: False"
	if got := en.renderCronJobSchedule(active); got != want {
		t.Errorf("en active schedule:\n got %q\nwant %q", got, want)
	}

	suspended := &model.CronJobData{Name: "cj-x", Namespace: "default", Schedule: "0 0 * * *", Suspend: true}
	want = "Schedule\n\n" +
		"  Schedule: 0 0 * * *\n" +
		"  Suspend: True (Job execution suspended)"
	if got := en.renderCronJobSchedule(suspended); got != want {
		t.Errorf("en suspended schedule:\n got %q\nwant %q", got, want)
	}
}

func TestRenderCronJobScheduleZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	suspended := &model.CronJobData{Name: "cj-x", Namespace: "default", Schedule: "0 0 * * *", Suspend: true}
	got := zh.renderCronJobSchedule(suspended)
	for _, noun := range []string{"调度计划", "暂停", "（任务执行已暂停）", "0 0 * * *"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh suspended schedule %q does not contain %q", got, noun)
		}
	}
	// The suspend boolean is a k8s value and stays untranslated on purpose.
	for _, bad := range []string{
		"Schedule:", "Suspend:", "Job execution suspended", "{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh suspended schedule %q contains %q", got, bad)
		}
	}
}

func TestRenderCronJobStatusEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	// Golden pins "1h", which holds while the fixture-to-render delay stays
	// under 30m — the -90m offset sits exactly at the lower edge of the
	// [1h,2h) window, so any real execution delay only moves deeper into it.
	_, full := cronJobFixtures(time.Now().Add(-5*time.Minute), time.Now().Add(-90*time.Minute))
	want := "Status\n\n" +
		"  Active Jobs: 0\n" +
		"  Last Schedule Time: 1h ago"
	if got := en.renderCronJobStatus(full); got != want {
		t.Errorf("en status with last schedule:\n got %q\nwant %q", got, want)
	}

	never := &model.CronJobData{Name: "cj-x", Namespace: "default"}
	want = "Status\n\n" +
		"  Active Jobs: 0\n" +
		"  Last Schedule Time: Never"
	if got := en.renderCronJobStatus(never); got != want {
		t.Errorf("en status never scheduled:\n got %q\nwant %q", got, want)
	}
}

func TestRenderCronJobStatusZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	_, full := cronJobFixtures(time.Now().Add(-5*time.Minute), time.Now().Add(-90*time.Minute))

	got := zh.renderCronJobStatus(full)
	for _, noun := range []string{"状态", "活跃任务", "上次调度时间：", " 前"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh status %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Status", "Active Jobs", "Last Schedule Time", " ago", "Never",
		"{{", "}}", "<no value", "%!", "detail.",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh status %q contains %q", got, bad)
		}
	}

	never := &model.CronJobData{Name: "cj-x", Namespace: "default"}
	got = zh.renderCronJobStatus(never)
	if !strings.Contains(got, "从未") {
		t.Errorf("zh status never %q does not contain 从未", got)
	}
	if strings.Contains(got, "Never") {
		t.Errorf("zh status never %q contains Never", got)
	}
}

func TestRenderCronJobSelectorEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)

	empty := en.renderCronJobSelector(&model.CronJobData{Name: "cj-x", Namespace: "default"})
	want := "Labels\n\n  No labels"
	if empty != want {
		t.Errorf("en zero-labels render:\n got %q\nwant %q", empty, want)
	}

	_, full := cronJobFixtures(time.Now(), time.Now())
	got := en.renderCronJobSelector(full)
	want = "Labels\n\n" +
		"  env: prod\n" +
		"  team: api"
	if got != want {
		t.Errorf("en labels render:\n got %q\nwant %q", got, want)
	}
}

func TestRenderCronJobSelectorZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)

	empty := zh.renderCronJobSelector(&model.CronJobData{Name: "cj-x", Namespace: "default"})
	for _, noun := range []string{"标签", "无标签"} {
		if !strings.Contains(empty, noun) {
			t.Errorf("zh zero-labels render %q does not contain %q", empty, noun)
		}
	}
	for _, bad := range []string{"Labels", "No labels", "{{", "}}", "<no value", "%!", "detail."} {
		if strings.Contains(empty, bad) {
			t.Errorf("zh zero-labels render %q contains %q", empty, bad)
		}
	}
}

func TestRenderCronJobActiveJobsEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	// Fixture times pin the "5m" age column; same reasoning as the basic-info
	// golden above.
	minimal, full := cronJobFixtures(time.Now().Add(-5*time.Minute), time.Now())

	// No cluster data at all.
	got := en.renderCronJobActiveJobs(minimal)
	want := "Active Jobs\n\n  No job information available"
	if got != want {
		t.Errorf("en no-cluster-data render:\n got %q\nwant %q", got, want)
	}

	// Related jobs covering every status branch; names must sort under "cj-x-".
	mk := func(name string, succeeded, failed, active, completions int32, dur time.Duration) *model.JobData {
		return &model.JobData{
			Name: name, Namespace: "default",
			Succeeded: succeeded, Failed: failed, Active: active, Completions: completions,
			Duration:          dur,
			CreationTimestamp: time.Now().Add(-5 * time.Minute),
		}
	}

	// Cluster data with jobs, but none related to this cronjob (the
	// empty-Jobs case falls into the no-information branch instead).
	en.clusterData = &model.ClusterData{Jobs: []*model.JobData{
		mk("other-job", 0, 0, 0, 1, 0),
	}}
	got = en.renderCronJobActiveJobs(minimal)
	want = "Related Jobs (0)\n\n  No jobs found for this cronjob"
	if got != want {
		t.Errorf("en no-jobs render:\n got %q\nwant %q", got, want)
	}

	en.clusterData = &model.ClusterData{Jobs: []*model.JobData{
		mk("cj-x-done", 3, 0, 0, 3, 4*time.Minute),
		mk("cj-x-fail", 1, 2, 0, 3, 2*time.Minute),
		mk("cj-x-run", 1, 0, 2, 3, 0),
		mk("cj-x-pend", 0, 0, 0, 3, 0),
	}}
	got = en.renderCronJobActiveJobs(full)
	want = "Related Jobs (4)\n\n" +
		"  " + "JOB NAME" + strings.Repeat(" ", 27) + "  " +
		"COMPLETIONS" + strings.Repeat(" ", 4) + "  " +
		"DURATION" + strings.Repeat(" ", 4) + "  " +
		"AGE" + strings.Repeat(" ", 7) + "  " +
		"STATUS" + strings.Repeat(" ", 6) + "\n" +
		"  " + "cj-x-done" + strings.Repeat(" ", 26) + "  " +
		"3/3" + strings.Repeat(" ", 12) + "  " +
		"4m0s" + strings.Repeat(" ", 8) + "  " +
		"5m" + strings.Repeat(" ", 8) + "  " +
		"Complete" + strings.Repeat(" ", 4) + "\n" +
		"  " + "cj-x-fail" + strings.Repeat(" ", 26) + "  " +
		"1/3" + strings.Repeat(" ", 12) + "  " +
		"2m0s" + strings.Repeat(" ", 8) + "  " +
		"5m" + strings.Repeat(" ", 8) + "  " +
		"Failed(2)" + strings.Repeat(" ", 3) + "\n" +
		"  " + "cj-x-run" + strings.Repeat(" ", 27) + "  " +
		"1/3" + strings.Repeat(" ", 12) + "  " +
		"running" + strings.Repeat(" ", 5) + "  " +
		"5m" + strings.Repeat(" ", 8) + "  " +
		"Active(2)" + strings.Repeat(" ", 3) + "\n" +
		"  " + "cj-x-pend" + strings.Repeat(" ", 26) + "  " +
		"0/3" + strings.Repeat(" ", 12) + "  " +
		"-" + strings.Repeat(" ", 11) + "  " +
		"5m" + strings.Repeat(" ", 8) + "  " +
		"Pending" + strings.Repeat(" ", 5)
	if got != want {
		t.Errorf("en related-jobs table:\n got %q\nwant %q", got, want)
	}

	// Thirteen related jobs: cap at ten rows plus the "... and N more" line.
	var many []*model.JobData
	for i := 0; i < 13; i++ {
		many = append(many, mk("cj-x-j"+strconv.Itoa(i), 0, 0, 1, 3, 0))
	}
	en.clusterData = &model.ClusterData{Jobs: many}
	got = en.renderCronJobActiveJobs(full)
	if !strings.Contains(got, "Related Jobs (13)") {
		t.Errorf("en many-jobs render %q does not contain the count header", got)
	}
	if !strings.HasSuffix(got, "  ... and 3 more jobs") {
		t.Errorf("en many-jobs render must end with the more-jobs line: %q", got)
	}
}

func TestRenderCronJobActiveJobsZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	minimal, full := cronJobFixtures(time.Now().Add(-5*time.Minute), time.Now())

	got := zh.renderCronJobActiveJobs(minimal)
	for _, noun := range []string{"活跃任务", "暂无任务信息"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-cluster-data render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Active Jobs", "No job information", "{{", "}}", "<no value", "%!", "detail."} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-cluster-data render %q contains %q", got, bad)
		}
	}

	mk := func(name string, succeeded, failed, active, completions int32, dur time.Duration) *model.JobData {
		return &model.JobData{
			Name: name, Namespace: "default",
			Succeeded: succeeded, Failed: failed, Active: active, Completions: completions,
			Duration:          dur,
			CreationTimestamp: time.Now().Add(-5 * time.Minute),
		}
	}

	// Jobs exist but none relate to this cronjob (empty Jobs falls into the
	// no-information branch instead).
	zh.clusterData = &model.ClusterData{Jobs: []*model.JobData{
		mk("other-job", 0, 0, 0, 1, 0),
	}}
	got = zh.renderCronJobActiveJobs(minimal)
	for _, noun := range []string{"关联任务（0）", "未找到该定时任务的关联任务"} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh no-jobs render %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{"Related Jobs", "No jobs found", "{{", "}}", "<no value", "%!"} {
		if strings.Contains(got, bad) {
			t.Errorf("zh no-jobs render %q contains %q", got, bad)
		}
	}

	zh.clusterData = &model.ClusterData{Jobs: []*model.JobData{
		mk("cj-x-done", 3, 0, 0, 3, 4*time.Minute),
		mk("cj-x-fail", 1, 2, 0, 3, 2*time.Minute),
		mk("cj-x-run", 1, 0, 2, 3, 0),
		mk("cj-x-pend", 0, 0, 0, 3, 0),
	}}
	got = zh.renderCronJobActiveJobs(full)
	for _, noun := range []string{
		"关联任务（4）", "任务名称", "完成情况", "持续时间", "年龄", "状态",
		"完成", "失败(2)", "活跃(2)", "等待中", "运行中",
	} {
		if !strings.Contains(got, noun) {
			t.Errorf("zh related-jobs table %q does not contain %q", got, noun)
		}
	}
	for _, bad := range []string{
		"Related Jobs", "JOB NAME", "COMPLETIONS", "DURATION", "AGE", "STATUS",
		"Complete", "Failed(", "Active(", "Pending", "running",
		"{{", "}}", "<no value", "%!",
	} {
		if strings.Contains(got, bad) {
			t.Errorf("zh related-jobs table %q contains %q", got, bad)
		}
	}

	var many []*model.JobData
	for i := 0; i < 13; i++ {
		many = append(many, mk("cj-x-j"+strconv.Itoa(i), 0, 0, 1, 3, 0))
	}
	zh.clusterData = &model.ClusterData{Jobs: many}
	got = zh.renderCronJobActiveJobs(full)
	if !strings.HasSuffix(got, "  ... 还有 3 个任务") {
		t.Errorf("zh many-jobs render must end with the more-jobs line: %q", got)
	}
	if strings.Contains(got, "more jobs") {
		t.Errorf("zh many-jobs render %q contains the English more-jobs text", got)
	}
}

// TestRenderCronJobDetailEnGolden locks the outer chrome: the nil-selection
// placeholder and the scroll indicator (short variant, no PgUp/PgDn hint).
func TestRenderCronJobDetailEnGolden(t *testing.T) {
	forcedAsciiProfile(t)

	en := NewModel(nil, zap.NewNop(), time.Second, "en", "test", 200)
	if got := en.renderCronJobDetail(); got != "No cronjob selected" {
		t.Errorf("en nil-selection = %q, want %q", got, "No cronjob selected")
	}

	_, full := cronJobFixtures(time.Now().Add(-5*time.Minute), time.Now())
	en.selectedCronJob = full
	en.clusterData = nil

	en.height = 1000
	tall := en.renderCronJobDetail()
	if strings.Contains(tall, "Lines 1-") {
		t.Errorf("tall render must not show the scroll indicator:\n%s", tall)
	}
	total := len(strings.Split(tall, "\n"))

	en.height = 12 // maxVisible = 2
	short := en.renderCronJobDetail()
	wantSuffix := "[Lines 1-2 of " + strconv.Itoa(total) + "] (↑/↓ to scroll)"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("short render must end with %q:\n%s", wantSuffix, short)
	}
}

func TestRenderCronJobDetailZh(t *testing.T) {
	forcedAsciiProfile(t)

	zh := NewModel(nil, zap.NewNop(), time.Second, "zh", "test", 200)
	if got := zh.renderCronJobDetail(); got != "未选择定时任务" {
		t.Errorf("zh nil-selection = %q, want 未选择定时任务", got)
	}

	_, full := cronJobFixtures(time.Now().Add(-5*time.Minute), time.Now())
	zh.selectedCronJob = full
	zh.clusterData = nil

	zh.height = 1000
	tall := zh.renderCronJobDetail()
	total := len(strings.Split(tall, "\n"))

	zh.height = 12
	short := zh.renderCronJobDetail()
	wantSuffix := "[第 1-2 行，共 " + strconv.Itoa(total) + " 行]（↑/↓ 滚动）"
	if !strings.HasSuffix(short, wantSuffix) {
		t.Errorf("zh short render must end with %q:\n%s", wantSuffix, short)
	}
	if strings.Contains(short, "↑/↓ to scroll") {
		t.Errorf("zh short render still contains the English scroll hint:\n%s", short)
	}
}
