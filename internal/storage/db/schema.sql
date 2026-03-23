CREATE TABLE IF NOT EXISTS documents (
    content_hash TEXT PRIMARY KEY,
    url TEXT UNIQUE,
    title TEXT,
    snippet TEXT,
    object_key TEXT,
    inbound_links INT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS inverted_index (
    term TEXT,
    content_hash TEXT,
    freq INT DEFAULT 1,

    PRIMARY KEY (term, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_term ON inverted_index(term);
CREATE INDEX IF NOT EXISTS idx_doc ON inverted_index(content_hash);
