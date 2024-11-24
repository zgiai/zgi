import requests

def call_ollama_api():
    # 调用 ollama api 获取数据
    response = requests.get("https://api.ollama.com")
    
    # 检查响应状态码
    if response.status_code == 200:
        # 解析 JSON 数据
        data = response.json()
        return data
    else:
        # 处理错误情况
        return None

def ollama_service():
    # 调用 ollama api 获取数据
    data = call_ollama_api()
    
    if data is not None:
        # 构建返回的 JSON 数据
        json_data = {
            "message": "Success",
            "data": data
        }
    else:
        # 构建返回的 JSON 数据
        json_data = {
            "message": "Error",
            "data": None
        }
    
    return json_data

# 测试 ollama_service
result = ollama_service()
print(result)
