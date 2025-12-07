-- cluster table stores the key-value pairs of the cluster configuration.
CREATE TABLE cluster
(
    key   TEXT NOT NULL PRIMARY KEY,
    value ANY
);

-- machines table stores the basic information of the machines in the cluster.
CREATE TABLE machines
(
    id   TEXT NOT NULL PRIMARY KEY,
    name TEXT AS (json_extract(info, '$.name')),
    -- info is a JSON-serialized MachineInfo protobuf message.
    info TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(info))
    -- TODO: add created_at and updated_at fields to track machine age and last update time.
);

-- containers table stores the Uncloud-managed Docker containers created in the cluster.
CREATE TABLE containers
(
    id           TEXT NOT NULL PRIMARY KEY,
    -- container is a JSON-serialized api.ServiceContainer struct.
    container    TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(container)),
    machine_id   TEXT NOT NULL DEFAULT '',
    service_id   TEXT AS (json_extract(container, '$.Config.Labels."uncloud.service.id"')),
    service_name TEXT AS (json_extract(container, '$.Config.Labels."uncloud.service.name"')),
    namespace    TEXT AS (coalesce(json_extract(container, '$.Config.Labels."uncloud.namespace"'), 'default')),
    -- sync_status indicates if the record reflects the actual Docker state of the container.
    sync_status  TEXT NOT NULL DEFAULT '',
    -- updated_at is the last time the record was updated.
    updated_at   TIMESTAMP NOT NULL DEFAULT '1970-01-01 00:00:00'
);

CREATE INDEX idx_machines_name ON machines (name);

CREATE INDEX idx_containers_machine_id ON containers (machine_id);
CREATE INDEX idx_containers_service_id ON containers (service_id);
CREATE INDEX idx_containers_service_name ON containers (service_name);
CREATE INDEX idx_containers_namespace ON containers (namespace);
