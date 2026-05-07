CREATE TABLE IF NOT EXISTS role_endpoints (
    role_id           INTEGER     NOT NULL REFERENCES roles(id),
    endpoint_identity VARCHAR(64) NOT NULL REFERENCES endpoints(identity),
    PRIMARY KEY (role_id, endpoint_identity)
);
