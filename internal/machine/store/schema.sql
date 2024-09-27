CREATE TABLE machines
(
    id   TEXT NOT NULL PRIMARY KEY,
    name TEXT AS (json_extract(info, '$.name')),
    info TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(info))
);

CREATE INDEX idx_machines_name ON machines (name);
