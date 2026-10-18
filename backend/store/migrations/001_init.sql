CREATE TABLE locations (
    id   text PRIMARY KEY,
    kind text NOT NULL,
    city text NOT NULL,
    name text NOT NULL
);

CREATE TABLE parcels (
    ttn             text PRIMARY KEY,
    sender_phone    text NOT NULL DEFAULT '',
    recipient_phone text NOT NULL DEFAULT '',
    origin_id       text NOT NULL,
    destination_id  text NOT NULL,
    created_at      timestamptz NOT NULL
);

CREATE TABLE route_legs (
    ttn               text NOT NULL,
    seq               int NOT NULL,
    from_id           text NOT NULL,
    to_id             text NOT NULL,
    trip_id           text NOT NULL,
    planned_departure timestamptz NOT NULL,
    planned_arrival   timestamptz NOT NULL,
    PRIMARY KEY (ttn, seq)
);
CREATE INDEX route_legs_to_id ON route_legs (to_id);
CREATE INDEX route_legs_trip_id ON route_legs (trip_id);

-- append-only journal: state is always derived from it
CREATE TABLE events (
    event_id    text PRIMARY KEY,
    kind        text NOT NULL,
    ttn         text,
    trip_id     text,
    location_id text,
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL,
    source      text NOT NULL,
    payload     jsonb
);
CREATE INDEX events_ttn ON events (ttn, occurred_at);
CREATE INDEX events_trip ON events (trip_id);

CREATE TABLE np_status (
    ttn                text PRIMARY KEY,
    status_code        text NOT NULL,
    status_text        text NOT NULL,
    scheduled_delivery timestamptz,
    fetched_at         timestamptz NOT NULL
);

CREATE TABLE audit_log (
    id        bigserial PRIMARY KEY,
    at        timestamptz NOT NULL,
    role      text NOT NULL,
    branch_id text NOT NULL DEFAULT '',
    action    text NOT NULL,
    ttn       text NOT NULL DEFAULT '',
    result    text NOT NULL
);

CREATE TABLE app_state (
    key   text PRIMARY KEY,
    value jsonb NOT NULL
);
