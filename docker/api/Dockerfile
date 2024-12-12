# 使用 Python 官方镜像
FROM python:3.12-slim

# 设置工作目录
WORKDIR /app

COPY ./api /app


# # # # 安装依赖
# RUN ls /app/requirements.txt
RUN pip install -r requirements.txt


# # 启动应用程序
CMD ["python3", "main.py"]