#!/usr/bin/env python3
#  -*- coding: utf-8 -*-
__author__ = 'chengzhi'

import os
import atexit
import time
import uuid

from datetime import datetime

from tqsdk import TqApi, TqAuth, TqKq
from tqsdk import TqAccount
from tqsdk import TargetPosTask

passwd = os.getenv("SHINNYTECH_PW")

simnow_user = os.getenv("SIMNOW_USER_ID")
simnow_pass = os.getenv("SIMNOW_USER_PASSWD")

def now_ms() -> str:
    # 本地时间，格式：2026-01-28 13:45:12.123
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]


def sample1():
    acc = TqAccount("simnow", "046056", "123@simnow")
    
    api = TqApi(account=acc, auth=TqAuth(
        "neuron", passwd), web_gui="0.0.0.0:8082", debug="debug.log",
        _td_url="ws://127.0.0.1:7788")
    
    # position = api.get_position("DCE.m2209")
    positions = api.get_position()
    # 获得资金账户引用，当账户有变化时 account 中的字段会对应更新
    accinfo = api.get_account()
    print(accinfo)
    print(positions)
    
    while True:
        api.wait_update()
        if api.is_changing(positions):
            for symbol, pos in positions.items():
                print(f"{now_ms()} {symbol} 持仓变化: 多浮动盈亏:{pos.float_profit_long} 空浮动盈亏:{pos.float_profit_short}")
    pass


if __name__ == '__main__':
    sample1()
    pass
