
#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import getpass
import json
import os
import re
import sys
from collections import defaultdict

try:
    import keyring
    import pexpect
except ImportError:
    print("Required libraries not found. Please install them:")
    print("pip install keyring pexpect")
    sys.exit(1)


# Use a constant for the service name in keyring
KEYRING_SERVICE_NAME = "server_manager_cli"
# Path for the configuration file in the user's home directory
CONFIG_FILE = os.path.expanduser("~/.server_manager.json")
# Default passwords list
DEFAULT_PASSWORDS = [
    "Uniontech@2023",
    "Uniontech@2024",
    "uos123..",
]


class ServerManager:
    """Manages server connection information."""

    def __init__(self):
        """Initializes the ServerManager and loads existing data."""
        self.servers = self._load_servers()

    def _load_servers(self):
        """Loads server data from the JSON config file."""
        if not os.path.exists(CONFIG_FILE):
            return {}
        try:
            with open(CONFIG_FILE, "r") as f:
                return json.load(f)
        except (json.JSONDecodeError, IOError):
            print(f"Warning: Could not read or parse {CONFIG_FILE}. Starting fresh.")
            return {}

    def _save_servers(self):
        """Saves the current server data to the JSON config file."""
        try:
            with open(CONFIG_FILE, "w") as f:
                json.dump(self.servers, f, indent=4)
        except IOError as e:
            print(f"Error: Could not save server data to {CONFIG_FILE}. {e}")
            sys.exit(1)

    def _get_keyring_username(self, server_name):
        """Creates a unique identifier for storing credentials in the keyring."""
        return f"{server_name}"

    def _set_password(self, key, password):
        """Stores a password securely in the system's keyring."""
        keyring.set_password(KEYRING_SERVICE_NAME, key, password)

    def _get_password(self, key):
        """Retrieves a password from the system's keyring."""
        return keyring.get_password(KEYRING_SERVICE_NAME, key)

    def _delete_password(self, key):
        """Deletes a password from the system's keyring."""
        try:
            keyring.delete_password(KEYRING_SERVICE_NAME, key)
        except keyring.errors.PasswordDeleteError:
            # This can happen if the password doesn't exist, which is fine.
            pass

    def add_server(self, name, group, host, user, password=None, bastion=None):
        """Adds a new server configuration.
        
        Args:
            name: Unique name for the server
            group: Group to organize servers
            host: Server hostname or IP
            user: SSH username
            password: Password (if provided, otherwise will prompt)
            bastion: Name of bastion/jump host server (optional)
        """
        if name in self.servers:
            print(f"Error: Server '{name}' already exists. Use a different name.")
            return
        
        # Validate bastion server exists if specified
        if bastion:
            if bastion not in self.servers:
                print(f"Error: Bastion server '{bastion}' not found. Please add it first.")
                return
            if bastion == name:
                print(f"Error: Server cannot use itself as bastion.")
                return

        password_key = self._get_keyring_username(name)
        
        # Check if we need to set a password
        if password:
            self._set_password(password_key, password)
            print(f"Password for '{name}' has been securely stored.")
        elif self._get_password(password_key) is None:
            new_password = getpass.getpass(f"Enter password for '{name}': ")
            self._set_password(password_key, new_password)
            print(f"Password for '{name}' has been securely stored.")

        self.servers[name] = {
            "group": group,
            "host": host,
            "user": user,
            "bastion": bastion
        }
        self._save_servers()
        bastion_msg = f" (via bastion '{bastion}')" if bastion else ""
        print(f"Successfully added server '{name}' to group '{group}'{bastion_msg}.")

    def add_server_interactive(self):
        """Interactive mode for adding a new server configuration."""
        print("\n=== 添加服务器 (交互模式) ===\n")
        
        # 1. Server name
        while True:
            name = input("服务器名称 (例如: prod_web): ").strip()
            if not name:
                print("错误: 服务器名称不能为空")
                continue
            if name in self.servers:
                print(f"错误: 服务器 '{name}' 已存在，请使用其他名称")
                continue
            break
        
        # 2. Group
        # Get existing groups
        existing_groups = set()
        for server_details in self.servers.values():
            existing_groups.add(server_details.get("group", "default"))
        
        if existing_groups:
            print("\n已有分组:")
            sorted_groups = sorted(existing_groups)
            for idx, grp in enumerate(sorted_groups, 1):
                # Count servers in this group
                count = sum(1 for s in self.servers.values() if s.get("group") == grp)
                print(f"  {idx}. {grp} ({count} 台服务器)")
            
            group_input = input(f"\n选择分组 (输入编号或新分组名称, 默认: default): ").strip()
            
            if not group_input:
                group = "default"
            elif group_input.isdigit():
                idx = int(group_input) - 1
                if 0 <= idx < len(sorted_groups):
                    group = sorted_groups[idx]
                else:
                    print("无效编号，使用默认分组 'default'")
                    group = "default"
            else:
                group = group_input
        else:
            group = input("分组 (例如: production, 默认: default): ").strip() or "default"
        
        # 3. Host
        while True:
            host = input("主机地址 (IP 或域名): ").strip()
            if host:
                break
            print("错误: 主机地址不能为空")
        
        # 4. User
        while True:
            user = input("用户名: ").strip()
            if user:
                break
            print("错误: 用户名不能为空")
        
        # 5. Password selection
        password = None
        print("\n密码选择:")
        print("  0. 自定义密码")
        for idx, pwd in enumerate(DEFAULT_PASSWORDS, 1):
            # 只显示密码的前几位和后几位
            masked_pwd = pwd[:4] + "*" * (len(pwd) - 8) + pwd[-4:] if len(pwd) > 8 else pwd
            print(f"  {idx}. {masked_pwd}")
        
        pwd_choice = input(f"\n选择密码 (0-{len(DEFAULT_PASSWORDS)}, 默认: 1): ").strip() or "1"
        
        if pwd_choice.isdigit():
            choice_idx = int(pwd_choice)
            if 1 <= choice_idx <= len(DEFAULT_PASSWORDS):
                password = DEFAULT_PASSWORDS[choice_idx - 1]
                print(f"已选择默认密码 #{choice_idx}")
            elif choice_idx == 0:
                # Custom password
                password = None  # Will prompt later in add_server
            else:
                print("无效选择，将使用默认密码 #1")
                password = DEFAULT_PASSWORDS[0]
        else:
            print("无效输入，将使用默认密码 #1")
            password = DEFAULT_PASSWORDS[0]
        
        # 6. Bastion
        bastion = None
        use_bastion = input("\n使用跳板机? (y/n, 默认: n): ").strip().lower()
        if use_bastion in ['y', 'yes']:
            # Show available servers as potential bastions
            if self.servers:
                print("\n可用的服务器 (可作为跳板机):")
                for idx, (srv_name, srv_details) in enumerate(sorted(self.servers.items()), 1):
                    print(f"  {idx}. {srv_name} ({srv_details['user']}@{srv_details['host']})")
                
                bastion_input = input("\n跳板机名称 (或输入编号): ").strip()
                
                # Check if input is a number
                if bastion_input.isdigit():
                    idx = int(bastion_input) - 1
                    server_list = sorted(self.servers.keys())
                    if 0 <= idx < len(server_list):
                        bastion = server_list[idx]
                    else:
                        print("警告: 无效的编号，跳过跳板机设置")
                else:
                    if bastion_input in self.servers:
                        bastion = bastion_input
                    elif bastion_input:
                        print(f"警告: 跳板机 '{bastion_input}' 不存在，跳过跳板机设置")
            else:
                print("提示: 当前没有可用的服务器作为跳板机")
        
        # 7. Confirmation
        print("\n=== 确认信息 ===")
        print(f"服务器名称: {name}")
        print(f"分组: {group}")
        print(f"主机: {host}")
        print(f"用户: {user}")
        if password:
            print(f"密码: {'默认密码' if password in DEFAULT_PASSWORDS else '自定义'}")
        else:
            print(f"密码: 自定义（稍后输入）")
        print(f"跳板机: {bastion if bastion else '无'}")
        
        confirm = input("\n确认添加? (y/n): ").strip().lower()
        if confirm not in ['y', 'yes']:
            print("已取消")
            return
        
        # Add the server
        self.add_server(name, group, host, user, password=password, bastion=bastion)

    def select_server_interactive(self, action="操作"):
        """Interactive server selection helper.
        
        Args:
            action: Description of the action to perform (e.g., "查看", "删除", "登录")
        
        Returns:
            Selected server name or None if cancelled
        """
        if not self.servers:
            print("没有配置的服务器。请先使用 'add' 命令添加服务器。")
            return None
        
        print(f"\n=== {action}服务器 ===\n")
        print("可用的服务器:")
        
        # Group servers by group for better display
        grouped_servers = defaultdict(list)
        for name, details in self.servers.items():
            grouped_servers[details.get("group", "Ungrouped")].append(name)
        
        server_list = []
        idx = 1
        for group, names in sorted(grouped_servers.items()):
            print(f"\n[{group}]")
            for name in sorted(names):
                details = self.servers[name]
                bastion_indicator = f" → via {details.get('bastion')}" if details.get('bastion') else ""
                print(f"  {idx}. {name} ({details['user']}@{details['host']}){bastion_indicator}")
                server_list.append(name)
                idx += 1
        
        while True:
            choice = input(f"\n选择服务器 (输入编号或名称, 'c' 取消): ").strip()
            
            if choice.lower() == 'c':
                print("已取消")
                return None
            
            # Check if input is a number
            if choice.isdigit():
                idx = int(choice) - 1
                if 0 <= idx < len(server_list):
                    return server_list[idx]
                else:
                    print("错误: 无效的编号")
            else:
                # Check if it's a server name
                if choice in self.servers:
                    return choice
                else:
                    print(f"错误: 服务器 '{choice}' 不存在")

    def parse_history(self, history_file="history.txt", auto_mode=False):
        """Parses history file to add new server configurations.

        Args:
            history_file (str): Path to the history file to parse. Defaults to "history.txt".
            auto_mode (bool): If True, automatically add servers without prompting. Defaults to False.
        """
        if not os.path.exists(history_file):
            print(f"Error: {history_file} not found.")
            return

        # Different regex patterns for various SSH command formats
        patterns = [
            # sshpass with single quotes
            r"sshpass -p'([^']*)' ssh(?:\s+-[^\s]*)*\s+(\S+)@(\S+)",
            # sshpass without quotes
            r"sshpass -p(\S+) ssh(?:\s+-[^\s]*)*\s+(\S+)@(\S+)",
            # Standard SSH with port
            r"ssh(?:\s+-[^\s]*)*\s+-p\s+(\d+)\s+(\S+)@(\S+)",
            # Standard SSH without port
            r"ssh(?:\s+-[^\s]*)*\s+(\S+)@(\S+)",
        ]

        if not auto_mode:
            group = input("Enter a group name for the servers from history: ")
        else:
            group = "Auto-imported"
            print(f"Auto mode enabled. Servers will be added to '{group}' group.")

        servers_found = []

        with open(history_file, "r", encoding="utf-8", errors="ignore") as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line:
                    continue

                # Try each pattern
                for pattern in patterns:
                    match = re.search(pattern, line)
                    if match:
                        groups = match.groups()

                        # Extract info based on pattern type
                        if "sshpass" in line:
                            # sshpass command with password
                            password = groups[0]
                            if len(groups) > 3:
                                # With port
                                port, user, host = groups
                            else:
                                # Without port
                                user, host = groups[1], groups[2]
                            port = None  # Port isn't used in current implementation
                        else:
                            # Standard SSH command (no password in command line)
                            password = None
                            if "-p" in line and len(groups) > 2:
                                # With port
                                port, user, host = groups
                            else:
                                # Without port
                                user, host = groups

                        # Clean up host (remove extra characters)
                        host = re.split(r'\s+', host)[0]  # Take only first part if there are options after
                        host = host.rstrip(';')  # Remove trailing semicolon if present

                        server_info = {
                            "user": user,
                            "host": host,
                            "password": password,
                            "line_number": line_num,
                            "line": line
                        }

                        # Check for duplicates
                        is_duplicate = any(s["user"] == user and s["host"] == host for s in servers_found)
                        if not is_duplicate:
                            servers_found.append(server_info)
                        break  # Found a match, no need to try other patterns

        if not servers_found:
            print("No SSH servers found in history file.")
            return

        print(f"\nFound {len(servers_found)} potential servers in history:")
        for i, server in enumerate(servers_found, 1):
            print(f"  {i}. {server['user']}@{server['host']} (line {server['line_number']})")

        if auto_mode:
            # In auto mode, add all servers that don't already exist
            added_count = 0
            skipped_count = 0

            for server in servers_found:
                # Generate server name
                server_name = f"{server['user']}@{server['host']}"

                # Check if server already exists
                if server_name in self.servers:
                    print(f"Skipping {server_name} - already exists.")
                    skipped_count += 1
                    continue

                # Add server
                try:
                    self.add_server(
                        server_name,
                        group,
                        server['host'],
                        server['user'],
                        password=server['password']
                    )
                    added_count += 1
                except Exception as e:
                    print(f"Failed to add {server_name}: {e}")

            print(f"\nAuto-import complete: {added_count} added, {skipped_count} skipped.")
        else:
            # Interactive mode
            print("\nOptions:")
            print("  a - Add all servers (you'll confirm each one)")
            print("  s - Select specific servers")
            print("  c - Cancel")

            choice = input("Choose an option (a/s/c): ").lower().strip()

            if choice == "c":
                print("Operation cancelled.")
                return
            elif choice == "a":
                # Add all with confirmation
                for server in servers_found:
                    server_name = f"{server['user']}@{server['host']}"

                    if server_name in self.servers:
                        print(f"Skipping {server_name} - already exists.")
                        continue

                    confirm = input(f"Add {server_name}? (y/n): ").lower().strip()
                    if confirm in ["y", "yes"]:
                        self.add_server(
                            server_name,
                            group,
                            server['host'],
                            server['user'],
                            password=server['password']
                        )
            elif choice == "s":
                # Select specific servers
                selections = input("Enter server numbers to add (comma separated, e.g., 1,3,5): ")
                try:
                    selected_indices = [int(x.strip()) - 1 for x in selections.split(",")]
                    for idx in selected_indices:
                        if 0 <= idx < len(servers_found):
                            server = servers_found[idx]
                            server_name = f"{server['user']}@{server['host']}"

                            if server_name in self.servers:
                                print(f"Skipping {server_name} - already exists.")
                                continue

                            self.add_server(
                                server_name,
                                group,
                                server['host'],
                                server['user'],
                                password=server['password']
                            )
                except ValueError:
                    print("Invalid selection. Please enter comma-separated numbers.")

    def list_servers(self):
        """Lists all saved servers, grouped by their assigned group."""
        if not self.servers:
            print("No servers configured. Use 'add' to add one.")
            return

        grouped_servers = defaultdict(list)
        for name, details in self.servers.items():
            grouped_servers[details.get("group", "Ungrouped")].append(name)

        for group, names in sorted(grouped_servers.items()):
            print(f"[{group}]")
            for name in sorted(names):
                details = self.servers[name]
                bastion_indicator = f" → via {details.get('bastion')}" if details.get('bastion') else ""
                print(f"  - {name} ({details['user']}@{details['host']}){bastion_indicator}")


    def show_server(self, name):
        """Displays the details for a specific server."""
        if name not in self.servers:
            print(f"Error: Server '{name}' not found.")
            return

        details = self.servers[name]
        password_key = self._get_keyring_username(name)
        password = self._get_password(password_key)

        print(f"Configuration for '{name}':")
        print(f"  Group: {details['group']}")
        print(f"  Host: {details['host']}")
        print(f"  User: {details['user']}")
        print(f"  Password stored: {'Yes' if password else 'No'}")
        
        # Show bastion information
        bastion = details.get('bastion')
        if bastion:
            print(f"  Bastion server: {bastion}")
            if bastion in self.servers:
                bastion_details = self.servers[bastion]
                print(f"    → {bastion_details['user']}@{bastion_details['host']}")


    def delete_server(self, name):
        """Deletes a server's configuration and its password."""
        if name not in self.servers:
            print(f"Error: Server '{name}' not found.")
            return

        # Delete the password
        self._delete_password(self._get_keyring_username(name))
        print(f"Securely deleted password for '{name}'.")

        del self.servers[name]
        self._save_servers()
        print(f"Successfully deleted server '{name}'.")

    def login_to_server(self, name):
        """Connects to a server via SSH using stored credentials.
        
        Supports connection through a bastion/jump host if configured.
        """
        if name not in self.servers:
            print(f"Error: Server '{name}' not found.")
            return

        details = self.servers[name]
        password_key = self._get_keyring_username(name)
        password = self._get_password(password_key)

        if not password:
            print(f"Error: No password found for '{name}'.")
            return

        host = details["host"]
        user = details["user"]
        bastion = details.get("bastion")
        
        # Check if we need to connect through a bastion
        if bastion:
            if bastion not in self.servers:
                print(f"Error: Bastion server '{bastion}' not found in configuration.")
                return
            
            bastion_details = self.servers[bastion]
            bastion_password_key = self._get_keyring_username(bastion)
            bastion_password = self._get_password(bastion_password_key)
            
            if not bastion_password:
                print(f"Error: No password found for bastion server '{bastion}'.")
                return
            
            bastion_host = bastion_details["host"]
            bastion_user = bastion_details["user"]
            
            print(f"Connecting to {user}@{host} via bastion {bastion_user}@{bastion_host}...")
            
            # Use SSH ProxyJump feature for cleaner connection
            ssh_command = f"ssh -o ProxyCommand='ssh -W %h:%p {bastion_user}@{bastion_host}' {user}@{host}"
            
            try:
                child = pexpect.spawn(ssh_command, timeout=10)
                
                # We may need to handle password prompts for both bastion and target
                passwords_entered = 0
                max_password_attempts = 2  # bastion + target
                
                while passwords_entered < max_password_attempts:
                    i = child.expect([
                        "password:", 
                        "Password:",
                        pexpect.EOF, 
                        pexpect.TIMEOUT,
                        "Are you sure you want to continue connecting (yes/no)?",
                        "Are you sure you want to continue connecting (yes/no/[fingerprint])?"
                    ], timeout=10)

                    if i == 0 or i == 1:  # Password prompt
                        # First password is for bastion, second is for target
                        if passwords_entered == 0:
                            child.sendline(bastion_password)
                        else:
                            child.sendline(password)
                        passwords_entered += 1
                        
                        # After entering the last password, give control to user
                        if passwords_entered >= max_password_attempts:
                            child.interact()
                            break
                    elif i == 2:  # EOF
                        print("Connection failed. Here is the output:")
                        print(child.before.decode())
                        break
                    elif i == 3:  # Timeout
                        # If we've entered passwords and timeout, probably connected
                        if passwords_entered > 0:
                            child.interact()
                        else:
                            print("Connection timed out.")
                        break
                    elif i == 4 or i == 5:  # New host key
                        print("Adding new host key...")
                        child.sendline("yes")
                        # Continue to next iteration to handle password prompt

            except pexpect.exceptions.ExceptionPexpect as e:
                print(f"An error occurred during SSH connection: {e}")
            except Exception as e:
                print(f"An unexpected error occurred: {e}")
        
        else:
            # Direct connection without bastion (original logic)
            ssh_command = f"ssh {user}@{host}"
            print(f"Connecting to {user}@{host}...")

            try:
                child = pexpect.spawn(ssh_command, timeout=10)
                # Handle different prompts for password
                i = child.expect([
                    "password:", 
                    "Password:",
                    pexpect.EOF, 
                    pexpect.TIMEOUT,
                    "Are you sure you want to continue connecting (yes/no)?"
                ])

                if i == 0 or i == 1:  # Password prompt
                    child.sendline(password)
                    child.interact()  # Give control to the user
                elif i == 2: # EOF
                    print("Connection failed. Here is the output:")
                    print(child.before.decode())
                elif i == 3: # Timeout
                    print("Connection timed out.")
                elif i == 4: # New host key
                    print("Adding new host key...")
                    child.sendline("yes")
                    child.expect(["password:", "Password:"])
                    child.sendline(password)
                    child.interact()

            except pexpect.exceptions.ExceptionPexpect as e:
                print(f"An error occurred during SSH connection: {e}")
            except Exception as e:
                print(f"An unexpected error occurred: {e}")



def main():
    """Main function to parse arguments and execute commands."""
    parser = argparse.ArgumentParser(
        description="SSH 服务器管理工具 - 安全管理和连接到多个服务器",
        formatter_class=argparse.RawTextHelpFormatter,
        epilog="""
使用示例:
  python server_manager.py add           # 添加服务器
  python server_manager.py list          # 列出所有服务器
  python server_manager.py show          # 查看服务器详情
  python server_manager.py delete        # 删除服务器
  python server_manager.py login         # 登录服务器
  python server_manager.py parse_history # 从历史记录导入服务器

所有命令都使用交互式模式，无需记忆参数。
        """
    )
    subparsers = parser.add_subparsers(dest="command", required=False)

    # 'add' command - 纯交互式
    subparsers.add_parser("add", help="添加新服务器 (交互式)")

    # 'list' command
    subparsers.add_parser("list", help="列出所有服务器")

    # 'show' command - 纯交互式
    subparsers.add_parser("show", help="查看服务器详情 (交互式)")

    # 'delete' command - 纯交互式
    subparsers.add_parser("delete", help="删除服务器 (交互式)")

    # 'login' command - 纯交互式
    subparsers.add_parser("login", help="登录到服务器 (交互式)")

    # 'parse_history' command
    parser_parse_history = subparsers.add_parser("parse_history", help="从历史记录导入服务器")
    parser_parse_history.add_argument("--file", help="历史文件路径 (默认: history.txt)")
    parser_parse_history.add_argument("--auto", action="store_true", help="自动导入模式")

    args = parser.parse_args()
    
    # If no command provided, show help
    if not args.command:
        parser.print_help()
        return
    
    manager = ServerManager()

    if args.command == "add":
        manager.add_server_interactive()
    elif args.command == "list":
        manager.list_servers()
    elif args.command == "show":
        server_name = manager.select_server_interactive("查看")
        if server_name:
            manager.show_server(server_name)
    elif args.command == "delete":
        server_name = manager.select_server_interactive("删除")
        if server_name:
            # Ask for confirmation
            confirm = input(f"\n确认删除服务器 '{server_name}'? (y/n): ").strip().lower()
            if confirm in ['y', 'yes']:
                manager.delete_server(server_name)
            else:
                print("已取消删除")
    elif args.command == "login":
        server_name = manager.select_server_interactive("登录")
        if server_name:
            manager.login_to_server(server_name)
    elif args.command == "parse_history":
        manager.parse_history(
            history_file=args.file if args.file else "history.txt",
            auto_mode=args.auto
        )


if __name__ == "__main__":
    main()
