package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mjl-/mox/mox-"
)

func TestOrganization(t *testing.T) {
	// Clean up before test
	os.RemoveAll("../testdata/store/data")
	mox.ConfigStaticPath = "../testdata/store/mox.conf"
	mox.MustLoadConfig(true, false)

	ctx := context.Background()

	// Initialize store (creates default org)
	err := Init(ctx)
	if err != nil {
		t.Fatalf("init: %s", err)
	}
	defer func() {
		err := Close()
		if err != nil {
			t.Fatalf("close: %s", err)
		}
	}()

	// Test 1: Default org is created
	org, err := OpenOrg(ctx, 1)
	if err != nil {
		t.Fatalf("open org 1: %s", err)
	}
	if org.ID != 1 {
		t.Errorf("expected org ID 1, got %d", org.ID)
	}
	if org.Slug != "default" {
		t.Errorf("expected slug 'default', got %q", org.Slug)
	}
	if org.Name != "Default" {
		t.Errorf("expected name 'Default', got %q", org.Name)
	}
	if org.SuspendedAt != nil {
		t.Errorf("expected suspendedAt to be nil")
	}
	if org.CreatedAt.IsZero() {
		t.Errorf("expected CreatedAt to be set")
	}

	// Test 2: OrgBySlug works
	org2, err := OrgBySlug(ctx, "default")
	if err != nil {
		t.Fatalf("org by slug: %s", err)
	}
	if org2.ID != org.ID {
		t.Errorf("expected same org from slug lookup")
	}

	// Test 3: OrgBySlug returns ErrAbsent for non-existent slug
	_, err = OrgBySlug(ctx, "nonexistent")
	if !errors.Is(err, ErrAbsent) {
		t.Errorf("expected ErrAbsent for non-existent slug, got %v", err)
	}

	// Test 4: Create a new org
	newOrg := &Organization{
		Name:      "Test Org",
		Slug:      "test-org",
		CreatedAt: time.Now(),
	}
	err = AuthDB.Insert(ctx, newOrg)
	if err != nil {
		t.Fatalf("insert org: %s", err)
	}
	if newOrg.ID == 0 {
		t.Errorf("expected non-zero ID after insert")
	}

	// Test 5: OpenOrg retrieves new org
	retrievedOrg, err := OpenOrg(ctx, newOrg.ID)
	if err != nil {
		t.Fatalf("open new org: %s", err)
	}
	if retrievedOrg.Name != "Test Org" {
		t.Errorf("expected name 'Test Org', got %q", retrievedOrg.Name)
	}
	if retrievedOrg.Slug != "test-org" {
		t.Errorf("expected slug 'test-org', got %q", retrievedOrg.Slug)
	}

	// Test 6: NormalizeOrgID
	if NormalizeOrgID(0) != DefaultOrgID {
		t.Errorf("expected NormalizeOrgID(0) = %d, got %d", DefaultOrgID, NormalizeOrgID(0))
	}
	if NormalizeOrgID(5) != 5 {
		t.Errorf("expected NormalizeOrgID(5) = 5, got %d", NormalizeOrgID(5))
	}

	// Test 7: WithOrgID and OrgIdFromContext
	ctxWithOrg := WithOrgID(ctx, 123)
	orgID, ok := OrgIdFromContext(ctxWithOrg)
	if !ok {
		t.Errorf("expected OrgIdFromContext to return ok=true")
	}
	if orgID != 123 {
		t.Errorf("expected OrgID 123, got %d", orgID)
	}

	// Test 8: OrgIdFromContext without OrgID
	_, ok = OrgIdFromContext(ctx)
	if ok {
		t.Errorf("expected OrgIdFromContext to return ok=false for context without org")
	}

	// Test 9: Verify default org is not recreated on second Init
	err = Init(ctx)
	if err == nil {
		// If Init succeeds (already initialized), that's fine - just check org still exists
		org, err := OpenOrg(ctx, 1)
		if err != nil {
			t.Fatalf("open org after second init: %s", err)
		}
		if org.Slug != "default" {
			t.Errorf("expected default org slug after second init, got %q", org.Slug)
		}
	} else if err.Error() != "already initialized" {
		t.Errorf("expected 'already initialized' error, got %v", err)
	}
}
