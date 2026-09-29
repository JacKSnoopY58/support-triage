-- Three-table schema for a small conversational service. Messages belong to a
-- conversation; each request owns one decision and its tool-call audit trail.
CREATE TABLE IF NOT EXISTS conversations (
    id text PRIMARY KEY,
    customer_json jsonb NOT NULL,
    messages_json jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(messages_json) = 'array'),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS requests (
    idempotency_key text PRIMARY KEY,
    payload_hash text NOT NULL,
    conversation_id text NOT NULL REFERENCES conversations(id),
    status text NOT NULL CHECK (status IN ('processing', 'completed', 'failed')),
    response_json jsonb,
    proposal_json jsonb,
    decision_json jsonb,
    tool_calls_json jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(tool_calls_json) = 'array'),
    lease_until timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS requests_conversation_time
    ON requests(conversation_id, created_at, idempotency_key);

-- The local record is written before calling the incident provider.
CREATE TABLE IF NOT EXISTS side_effect_attempts (
    operation_key text PRIMARY KEY,
    conversation_id text NOT NULL REFERENCES conversations(id),
    request_key text NOT NULL REFERENCES requests(idempotency_key),
    status text NOT NULL CHECK (status IN ('pending', 'completed', 'unknown', 'failed')),
    external_id text,
    error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS attempts_conversation_time
    ON side_effect_attempts(conversation_id, created_at);
