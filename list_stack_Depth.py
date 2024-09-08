# -*- coding:utf-8 -*-

import re

path = r"C:\Users\alexz\Documents\WXWork\1688854835296791\Cache\File\2024-09\vjdump\jstack-25247-20240906152919.log";
INIT = 0
QUOTE = 1
pattern = r'"([^"]*)"'
stack_map = {}
state = 0
with open(path) as jstack:
    thread_name = None
    for line in jstack:
        if line.startswith("\""):
            thread_names = re.findall(pattern, line)
            if thread_names:
                thread_name = thread_names[0]
                stack_map[thread_name] = []
        else:
            stack_map[thread_name].append(line)


for thread_name, stack_lines in sorted(stack_map.items(), key=lambda item: len(item[1]), reverse=True):
    print(f"Thread: {thread_name}, Stack Depth: {len(stack_lines)}")
