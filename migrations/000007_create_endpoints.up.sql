CREATE TABLE IF NOT EXISTS endpoints (
    identity VARCHAR(64)  PRIMARY KEY,
    path     VARCHAR(255) NOT NULL,
    method   VARCHAR(10)  NOT NULL,
    module   VARCHAR(64),
    kind     VARCHAR(64),
    remark   VARCHAR(255),
    UNIQUE (path, method)
);
