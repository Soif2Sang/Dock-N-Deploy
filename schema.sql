-- Table des projets (Repositories)
CREATE TABLE projects (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    repo_url TEXT NOT NULL UNIQUE,
    access_token TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Table des pipelines (Une exécution du fichier .gitlab-ci.yml)
CREATE TABLE pipelines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    status TEXT DEFAULT 'pending', -- pending, running, success, failed, cancelled
    commit_hash TEXT,              -- Le hash du commit qui a déclenché la pipeline
    branch TEXT,                   -- La branche concernée (ex: main)
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME,
    FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
);

-- Table des jobs (Les tâches individuelles dans une pipeline)
CREATE TABLE jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    pipeline_id INTEGER NOT NULL,
    name TEXT NOT NULL,            -- ex: build_job
    stage TEXT NOT NULL,           -- ex: build, test
    image TEXT NOT NULL,           -- ex: alpine:latest
    status TEXT DEFAULT 'pending', -- pending, running, success, failed
    exit_code INTEGER,             -- Code de retour du conteneur (0 = succès)
    started_at DATETIME,
    finished_at DATETIME,
    FOREIGN KEY(pipeline_id) REFERENCES pipelines(id) ON DELETE CASCADE
);

-- Table des logs (Stockage unitaire ligne par ligne pour le streaming)
CREATE TABLE logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id INTEGER NOT NULL,
    content TEXT,                  -- Le contenu de la ligne de log
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP, -- Pour trier les logs dans l'ordre
    FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE CASCADE
);

-- Index pour optimiser les requêtes fréquentes
CREATE INDEX idx_pipelines_project_id ON pipelines(project_id);
CREATE INDEX idx_jobs_pipeline_id ON jobs(pipeline_id);
CREATE INDEX idx_logs_job_id ON logs(job_id); -- Crucial pour récupérer les logs rapidement
