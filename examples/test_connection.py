#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
测试连接脚本 - 快速测试 WebSocket 服务是否可用

安装: pip install websockets
运行: python test_connection.py
"""

import asyncio
import sys

try:
    import websockets
except ImportError:
    print("错误: 未安装 websockets 库")
    print("请运行: pip install websockets")
    sys.exit(1)


async def test_connection(host="localhost", port=7788, timeout=5):
    """测试 WebSocket 连接"""
    url = f"ws://{host}:{port}/"
    
    print("=" * 50)
    print("  WebSocket 连接测试")
    print("=" * 50)
    print(f"\n目标地址: {url}")
    print(f"超时时间: {timeout} 秒")
    print("\n测试中...")
    
    try:
        # 尝试连接
        websocket = await asyncio.wait_for(
            websockets.connect(url),
            timeout=timeout
        )
        
        print("\n✓ 连接成功!")
        print(f"  - 本地地址: {websocket.local_address}")
        print(f"  - 远程地址: {websocket.remote_address}")
        
        # 发送 ping
        print("\n发送 ping...")
        pong_waiter = await websocket.ping()
        latency = await asyncio.wait_for(pong_waiter, timeout=timeout)
        print(f"✓ 收到 pong (延迟约 {latency*1000:.2f} ms)")
        
        # 关闭连接
        await websocket.close()
        print("\n✓ 测试完成，连接已关闭")
        
        return True
        
    except asyncio.TimeoutError:
        print(f"\n✗ 连接超时 (超过 {timeout} 秒)")
        print("\n可能的原因:")
        print("  1. 交易网关未启动")
        print("  2. 地址或端口配置错误")
        print("  3. 防火墙阻止连接")
        return False
        
    except ConnectionRefusedError:
        print("\n✗ 连接被拒绝")
        print("\n可能的原因:")
        print("  1. 交易网关未启动")
        print(f"  2. 端口 {port} 未被监听")
        print("  3. 检查配置文件中的 host 和 port 设置")
        return False
        
    except Exception as e:
        print(f"\n✗ 连接失败: {e}")
        print(f"  错误类型: {type(e).__name__}")
        return False


def print_help():
    """打印帮助信息"""
    print("\n" + "=" * 50)
    print("  帮助信息")
    print("=" * 50)
    print("\n启动交易网关:")
    print("  cd alpha-trade-gateway")
    print("  ./trader -config ./config/config.json")
    print("\n检查配置文件:")
    print("  cat alpha-trade-gateway/config/config.json")
    print("  确认 host 和 port 设置正确")
    print("\n检查进程:")
    print("  ps aux | grep trader")
    print("\n检查端口:")
    print("  lsof -i :7788")
    print("  netstat -an | grep 7788")
    print()


async def main():
    """主函数"""
    # 默认配置
    host = "localhost"
    port = 7788
    
    # 从命令行读取参数
    if len(sys.argv) > 1:
        if sys.argv[1] in ["-h", "--help", "help"]:
            print_help()
            return
        host = sys.argv[1]
    
    if len(sys.argv) > 2:
        try:
            port = int(sys.argv[2])
        except ValueError:
            print(f"错误: 端口必须是数字，得到: {sys.argv[2]}")
            sys.exit(1)
    
    # 执行测试
    success = await test_connection(host, port)
    
    # 显示结果
    if success:
        print("\n" + "=" * 50)
        print("  测试结果: 成功 ✓")
        print("=" * 50)
        print("\n可以继续使用 Python 客户端连接:")
        print("  python python_client.py")
        print("  python simple_example.py")
        sys.exit(0)
    else:
        print("\n" + "=" * 50)
        print("  测试结果: 失败 ✗")
        print("=" * 50)
        print_help()
        sys.exit(1)


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        print("\n\n测试中断")
        sys.exit(130)
