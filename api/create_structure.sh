#!/bin/bash

# Create main directory structure
mkdir -p app/{api/v1/endpoints,core,db,models/domain,schemas,services,utils}
mkdir -p tests/{api,services}
mkdir -p alembic/versions
mkdir -p {docs,scripts}

# Create files in app directory
touch app/main.py
touch app/api/deps.py
touch app/api/v1/router.py
touch app/api/v1/endpoints/{data_support,multi_modal,agent,vectorization,search,model_support}.py
touch app/core/{config,security}.py
touch app/db/{base,session}.py
touch app/services/{data_support,multi_modal,agent,vectorization,search,model_support}.py

# Create root level files
touch {.env,.gitignore,requirements.txt,requirements-dev.txt,Dockerfile,docker-compose.yml,README.md}

echo "Directory structure created successfully!"