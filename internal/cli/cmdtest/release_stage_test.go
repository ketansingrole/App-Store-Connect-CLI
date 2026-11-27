package cmdtest

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asctest"
	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/cli/shared/errfmt"
)

func TestReleaseStageUsageErrors(t *testing.T) {
	runUsageExitCases(t, []usageExitCase{
		{
			name:    "MissingMetadataSource",
			args:    []string{"release", "stage", "--app", "APP_123", "--version", "1.2.3", "--build-id", "BUILD_123", "--dry-run"},
			wantErr: "exactly one of --metadata-dir or --copy-metadata-from is required",
			deps:    true,
		},
		{
			name:    "InvalidCopyFieldsValue",
			args:    []string{"release", "stage", "--app", "APP_123", "--version", "1.2.3", "--build-id", "BUILD_123", "--copy-metadata-from", "1.2.2", "--copy-fields", "description,notAField", "--dry-run"},
			wantErr: "--copy-fields must be one of",
			deps:    true,
		},
	})
}

func TestReleaseStage_DryRunCopyMetadataFromVersion(t *testing.T) {
	setupAuth(t)

	reg := asctest.NewRegistry(t)
	reg.Register(asctest.GET("/v1/builds/BUILD_123/relationships/app"), asctest.JSON(http.StatusOK, `{"data":{"type":"apps","id":"APP_123"}}`))
	reg.Register(asctest.GET("/v1/builds/BUILD_123"), asctest.JSON(http.StatusOK, `{"data":{"type":"builds","id":"BUILD_123","attributes":{"version":"42","buildAudienceType":"APP_STORE_ELIGIBLE"}}}`))
	reg.Register(asctest.GET("/v1/builds/BUILD_123/preReleaseVersion"), asctest.JSON(http.StatusOK, `{"data":{"type":"preReleaseVersions","id":"PRE_RELEASE_123","attributes":{"version":"1.2.3","platform":"IOS"}}}`))
	reg.Register(asctest.GET("/v1/apps/APP_123/appStoreVersions", "filter[platform]=IOS", "filter[versionString]=1.2.3", "limit=10"), asctest.JSON(http.StatusOK, `{"data":[]}`))

	stdout, stderr, err := runRootWithDeps(t, stubClient(t, reg), []string{
		"release", "stage",
		"--app", "APP_123",
		"--version", "1.2.3",
		"--build-id", "BUILD_123",
		"--copy-metadata-from", "1.2.2",
		"--copy-fields", "description,keywords",
		"--exclude-fields", "keywords",
		"--dry-run",
	}...)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, `"status":"dry-run"`) {
		t.Fatalf("expected dry-run status, got %q", stdout)
	}
	if !strings.Contains(stdout, `"copyMetadataFrom":"1.2.2"`) {
		t.Fatalf("expected copy metadata source in output, got %q", stdout)
	}
	if !strings.Contains(stdout, `"message":"would create app store version"`) {
		t.Fatalf("expected ensure version dry-run message, got %q", stdout)
	}
	if !strings.Contains(stdout, `"message":"metadata copy plan deferred until version exists"`) {
		t.Fatalf("expected deferred metadata copy message, got %q", stdout)
	}
	if !strings.Contains(stdout, `"name":"validate_build","status":"dry-run"`) {
		t.Fatalf("expected the build precondition check in the plan, got %q", stdout)
	}
	if requestCount := len(reg.Requests()); requestCount != 4 {
		t.Fatalf("expected the build ownership, platform, and audience checks plus the version lookup, got %d requests", requestCount)
	}
}

// TestReleaseStage_ResolvesTrimmedCheckpointFile pins the CLI surface for
// --checkpoint-file: like every other path flag it is trimmed, so the reported
// and used checkpoint path never carries the operator's surrounding whitespace.
func TestReleaseStage_ResolvesTrimmedCheckpointFile(t *testing.T) {
	setupAuth(t)

	workDir := t.TempDir()
	t.Chdir(workDir)

	reg := asctest.NewRegistry(t)
	reg.Register(asctest.GET("/v1/builds/BUILD_123/relationships/app"), asctest.JSON(http.StatusOK, `{"data":{"type":"apps","id":"APP_123"}}`))
	reg.Register(asctest.GET("/v1/builds/BUILD_123"), asctest.JSON(http.StatusOK, `{"data":{"type":"builds","id":"BUILD_123","attributes":{"version":"42","buildAudienceType":"APP_STORE_ELIGIBLE"}}}`))
	reg.Register(asctest.GET("/v1/builds/BUILD_123/preReleaseVersion"), asctest.JSON(http.StatusOK, `{"data":{"type":"preReleaseVersions","id":"PRE_RELEASE_123","attributes":{"version":"1.2.3","platform":"IOS"}}}`))
	reg.Register(asctest.GET("/v1/apps/APP_123/appStoreVersions", "filter[platform]=IOS", "filter[versionString]=1.2.3", "limit=10"), asctest.JSON(http.StatusOK, `{"data":[]}`))

	stdout, _, err := runRootWithDeps(t, stubClient(t, reg), []string{
		"release", "stage",
		"--app", "APP_123",
		"--version", "1.2.3",
		"--build-id", "BUILD_123",
		"--copy-metadata-from", "1.2.2",
		"--checkpoint-file", "  stage-checkpoint.json  ",
		"--dry-run",
	}...)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	var plan struct {
		CheckpointFile string `json:"checkpointFile"`
	}
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("parse plan output %q: %v", stdout, err)
	}
	if !filepath.IsAbs(plan.CheckpointFile) {
		t.Fatalf("expected an absolute checkpoint path, got %q", plan.CheckpointFile)
	}
	if filepath.Base(plan.CheckpointFile) != "stage-checkpoint.json" {
		t.Fatalf("expected the checkpoint file name to be trimmed, got %q", plan.CheckpointFile)
	}
}

// TestReleaseStage_ExplainsRefusedVersionCreate: App Store Connect refuses to
// create a version while another of the platform is still in progress, here
// one App Review returned, and names neither (2026-11-27). The error names it
// and how to go on, and nothing else is written.
func TestReleaseStage_ExplainsRefusedVersionCreate(t *testing.T) {
	setupAuth(t)
	t.Chdir(t.TempDir())

	reg := asctest.NewRegistry(t)
	reg.Register(asctest.GET("/v1/builds/BUILD_123/relationships/app"), asctest.JSON(http.StatusOK, `{"data":{"type":"apps","id":"APP_123"}}`))
	reg.Register(asctest.GET("/v1/builds/BUILD_123"), asctest.JSON(http.StatusOK, `{"data":{"type":"builds","id":"BUILD_123","attributes":{"version":"42","buildAudienceType":"APP_STORE_ELIGIBLE"}}}`))
	reg.Register(asctest.GET("/v1/builds/BUILD_123/preReleaseVersion"), asctest.JSON(http.StatusOK, `{"data":{"type":"preReleaseVersions","id":"PRE_RELEASE_123","attributes":{"version":"1.2.4","platform":"IOS"}}}`))
	reg.Register(asctest.GET("/v1/apps/APP_123/appStoreVersions", "filter[platform]=IOS", "filter[versionString]=1.2.4", "limit=10"), asctest.JSON(http.StatusOK, `{"data":[]}`))
	reg.Register(asctest.POST("/v1/appStoreVersions"), asctest.JSON(http.StatusConflict, `{"errors":[{"status":"409","code":"ENTITY_ERROR.RELATIONSHIP.INVALID","title":"The provided entity includes a relationship with an invalid value","detail":"You cannot create a new version of the App in the current state."}]}`))
	reg.Register(asctest.GET("/v1/apps/APP_123/appStoreVersions", "filter[platform]=IOS", "limit=200"), asctest.JSON(http.StatusOK, `{"data":[{"type":"appStoreVersions","id":"VERSION_123","attributes":{"platform":"IOS","versionString":"1.2.3","appVersionState":"REJECTED","appStoreState":"REJECTED"}}]}`))
	reg.Register(asctest.GET("/v1/apps/APP_123/reviewSubmissions", "filter[platform]=IOS", "filter[state]=UNRESOLVED_ISSUES", "include=appStoreVersionForReview", "limit=200"), asctest.JSON(http.StatusOK, `{"data":[{"type":"reviewSubmissions","id":"SUBMISSION_123","attributes":{"state":"UNRESOLVED_ISSUES","platform":"IOS"},"relationships":{"appStoreVersionForReview":{"data":{"type":"appStoreVersions","id":"VERSION_123"}}}}],"links":{"self":"/v1/apps/APP_123/reviewSubmissions"}}`))
	reg.Register(asctest.GET("/v1/reviewSubmissions/SUBMISSION_123/items", "include=appStoreVersion", "limit=200"), asctest.JSON(http.StatusOK, `{"data":[{"type":"reviewSubmissionItems","id":"ITEM_123","attributes":{"state":"REJECTED"},"relationships":{"appStoreVersion":{"data":{"type":"appStoreVersions","id":"VERSION_123"}}}}],"links":{"self":"/v1/reviewSubmissions/SUBMISSION_123/items"}}`))

	_, _, err := runRootWithDeps(t, stubClient(t, reg), []string{
		"release", "stage",
		"--app", "APP_123",
		"--version", "1.2.4",
		"--build-id", "BUILD_123",
		"--copy-metadata-from", "1.2.2",
		"--confirm",
	}...)
	if err == nil {
		t.Fatal("expected the refused version create to fail the stage")
	}
	if !strings.Contains(err.Error(), "You cannot create a new version of the App in the current state.; version 1.2.3 (REJECTED) of IOS is still in progress") {
		t.Errorf("expected App Store Connect's refusal naming 1.2.3, got %v", err)
	}
	hint := errfmt.Classify(err).Hint
	for _, want := range []string{
		`asc review submissions-submit --id "SUBMISSION_123" --confirm`,
		`asc versions update --version-id "VERSION_123" --version "1.2.4"`,
	} {
		if !strings.Contains(hint, want) {
			t.Errorf("expected %q in the hint, got %q", want, hint)
		}
	}
}
