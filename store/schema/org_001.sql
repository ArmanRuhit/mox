CREATE TABLE organization (
    id bigint PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    name text NOT NULL,
    slug text NOT NULL UNIQUE,
    created_at timestampz NOT NULL,
    suspended_at timestampz
);