package store

import (
	"context"
	"time"
)

type Organization struct {
	ID int64
	Name string
	Slug string
	CreatedAt time.Time
	SuspendedAt *time.Time
}

// contextKey is a typed key for storing OrgID in context
// using a custom type prevents collissions with other packages
type contextKey int
const ctxOrgIdKey contextKey = iota
const DefaultOrgID int64 = 1

func OpenOrg(ctx context.Context, id int64) (*Organization, error)  {
	org := Organization{ID : id}

	err := AuthDB.Get(ctx, &org)

	if err != nil {
		return nil, err
	}

	return &org, nil
}

func OrgBySlug(ctx context.Context, slug string) (*Organization, error)  {
	orgs, err := QueryDB[Organization](ctx, AuthDB).FilterEqual("Slug", slug).List()

	if err != nil {
		return nil, err
	}

	if len(orgs) == 0 {
		return nil, ErrAbsent
	}

	return &orgs[0], nil
}

// WithOrgD returns a new context
func WithOrgID(ctx context.Context, orgId int64) context.Context {
	return context.WithValue(ctx, ctxOrgIdKey, orgId)
}


func OrgIdFromContext(ctx context.Context) (int64, bool)  {
	val := ctx.Value(ctxOrgIdKey)

	if val == nil {
		return 0, false
	}

	if id, ok := val.(int64); ok {
		return id, true
	}

	return 0, false
}

func NormalizeOrgID(orgID int64) int64 {
	if orgID == 0 {
		return DefaultOrgID
	}
	return orgID
}