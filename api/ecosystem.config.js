module.exports = {
  apps: [
    {
      name: 'zgi-backend', // 应用名称
      script: 'main.py', // 要运行的脚本
      interpreter: 'python3', // 指定 Python 解释器
      instances: '2', // 使用集群模式，实例数为 CPU 核心数
      exec_mode: 'cluster', // 启用集群模式
      watch: false, // 是否监视文件变化
      env: {
        NODE_ENV: 'production', // 设置环境变量
      },
    },
  ],
};
