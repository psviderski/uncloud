-- cluster table stores the key-value pairs of the cluster configuration.
CREATE TABLE cluster
(
    key   TEXT NOT NULL PRIMARY KEY,
    value ANY
);

CREATE TABLE machines
(
    id   TEXT NOT NULL PRIMARY KEY,
    name TEXT AS (json_extract(info, '$.name')),
    info TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(info))
);

CREATE TABLE containers
(
    id           TEXT NOT NULL PRIMARY KEY,
    container    TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(container)),
    machine_id   TEXT NOT NULL DEFAULT '',
    service_id   TEXT AS (json_extract(container, '$.Labels."uncloud.service.id"')),
    service_name TEXT AS (json_extract(container, '$.Labels."uncloud.service.name"'))
);

CREATE INDEX idx_machines_name ON machines (name);
CREATE INDEX idx_containers_service_id ON containers (service_id);
CREATE INDEX idx_containers_service_name ON containers (service_name);
