# 使用官方Python镜像作为基础镜像
FROM python:3.9-slim

# 设置工作目录
WORKDIR /app

# 复制requirements.txt（如果存在）
COPY requirements.txt .

# 更新 pip
RUN pip install --upgrade pip

# 安装项目依赖
RUN pip install --no-cache-dir -r requirements.txt -v

# 将源代码复制到工作目录
COPY . .

# 设置Python路径
ENV PYTHONPATH=/app

# 暴露应用程序运行的端口（根据您的应用需要更改）
EXPOSE 7001

# 定义运行应用程序的命令
CMD ["python", "main.py"]
