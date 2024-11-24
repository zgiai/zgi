import subprocess
import sys

def update_requirements():
    try:
        # 运行 pip freeze 命令并捕获输出
        result = subprocess.run([sys.executable, "-m", "pip", "freeze"], capture_output=True, text=True, check=True)
        
        # 将输出写入 requirements.txt 文件
        with open("requirements.txt", "w") as f:
            f.write(result.stdout)
        
        print("requirements.txt has been updated successfully.")
    except subprocess.CalledProcessError as e:
        print(f"An error occurred while running pip freeze: {e}")
    except IOError as e:
        print(f"An error occurred while writing to requirements.txt: {e}")

if __name__ == "__main__":
    update_requirements()
