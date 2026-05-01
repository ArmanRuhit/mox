CREATE TABLE organization (
    id bigint PRIMARY_KEY,
    name text NOT NULL,
    slug text NOT NULL UNIQUE,
    created_at timestampz NOT NULL,
    suspended_at timestampz
);