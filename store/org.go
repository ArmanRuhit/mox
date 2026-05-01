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