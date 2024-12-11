# ZGI AI App

A powerful AI application built with FastAPI, featuring vector search capabilities using Pinecone and Weaviate.

## Prerequisites

- Python 3.11+
- MySQL
- Pinecone account
- Weaviate account
- OpenAI API key

## Environment Setup

1. Clone the repository:
```bash
git clone <repository-url>
cd zgi/api
```

2. Create and activate a virtual environment:
```bash
python -m venv venv
source venv/bin/activate  # On Windows, use: venv\Scripts\activate
```

3. Install dependencies:
```bash
pip install -r requirements.txt
```

4. Set up environment variables by copying the example file:
```bash
cp .env.example .env
```

5. Update the `.env` file with your credentials:
```
OPENAI_API_KEY=your_openai_api_key
PINECONE_API_KEY=your_pinecone_api_key
PINECONE_ENVIRONMENT=your_pinecone_environment
PINECONE_INDEX_NAME=your_pinecone_index_name
WEAVIATE_URL=your_weaviate_url

# Database settings
DB_CONNECTION=mysql
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=your_database_name
DB_USERNAME=your_username
DB_PASSWORD=your_password
DB_CHARSET=utf8mb4
DB_COLLATION=utf8mb4_unicode_ci
```

## Database Initialization

1. Create the database:
```bash
mysql -u root -p
CREATE DATABASE your_database_name;
```

2. Set up Python path (Important for migrations to work):
```bash
# For macOS/Linux
export PYTHONPATH=/path/to/your/zgi/api:$PYTHONPATH

# For Windows
set PYTHONPATH=C:\path\to\your\zgi\api;%PYTHONPATH%
```

3. Initialize the database schema:
```bash
alembic upgrade head
```

4. Run the table creation script:
```bash
python create_tables.py
```

## Running the Application

1. Make sure your PYTHONPATH is set correctly:
```bash
# For macOS/Linux
export PYTHONPATH=/path/to/your/zgi/api:$PYTHONPATH
```

2. Start the application:
```bash
python run.py
```

Or use the start script:
```bash
./start.sh
```

The API will be available at `http://localhost:8000`

## API Documentation

Once the application is running, you can access:
- Swagger UI documentation: `http://localhost:8000/docs`
- ReDoc documentation: `http://localhost:8000/redoc`

## Testing

Run the tests using:
```bash
pytest
```

## Docker Deployment

To run the application using Docker:

```bash
docker-compose up -d
```

## Project Structure

- `app/`: Main application code
  - `api/`: API endpoints
  - `core/`: Core functionality and configurations
  - `models/`: Database models
  - `schemas/`: Pydantic schemas
- `alembic/`: Database migrations
- `tests/`: Test files
- `docs/`: Documentation files

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a new Pull Request