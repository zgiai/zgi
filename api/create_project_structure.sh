#!/bin/bash

# 创建目录结构
mkdir -p app/api/v1/endpoints

# 创建主入口文件
cat > main.py << EOL
from fastapi import FastAPI
from app.api.v1.router import router as api_v1_router

app = FastAPI()
app.include_router(api_v1_router, prefix="/api/v1")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run("main:app", host="0.0.0.0", port=7001, reload=True)
EOL

# 创建路由文件
cat > app/api/v1/router.py << EOL
from fastapi import APIRouter
from app.api.v1.endpoints import users, items, auth

router = APIRouter()

router.include_router(users.router, prefix="/users", tags=["users"])
router.include_router(items.router, prefix="/items", tags=["items"])
router.include_router(auth.router, prefix="/auth", tags=["auth"])
EOL

# 创建 endpoints/__init__.py
touch app/api/v1/endpoints/__init__.py

# 创建 endpoints 文件
for endpoint in users items auth; do
    cat > app/api/v1/endpoints/${endpoint}.py << EOL
from fastapi import APIRouter

router = APIRouter()

@router.get("/")
async def read_${endpoint}():
    return {"message": "This is the ${endpoint} endpoint"}
EOL
done

echo "Project structure created successfully!"