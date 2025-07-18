#!/usr/bin/env python3
"""
Maintenance update script to automate common tasks for maintenance updates across different versions of OCP
"""

import argparse
import os
import subprocess
import sys
import logging
import time
from datetime import datetime

logger = logging.getLogger(__name__)

class Colors:
    """ANSI color codes for terminal output"""
    RED = '\033[91m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    BLUE = '\033[94m'
    MAGENTA = '\033[95m'
    CYAN = '\033[96m'
    WHITE = '\033[97m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'
    RESET = '\033[0m'
    
    @classmethod
    def disable_colors(cls):
        """Disable colors for non-terminal output"""
        cls.RED = cls.GREEN = cls.YELLOW = cls.BLUE = ''
        cls.MAGENTA = cls.CYAN = cls.WHITE = cls.BOLD = ''
        cls.UNDERLINE = cls.RESET = ''

# Disable colors if not running in a terminal
if not sys.stdout.isatty():
    Colors.disable_colors()

def print_header(title, char='='):
    """Print a formatted header"""
    width = 60
    print(f"\n{Colors.CYAN}{Colors.BOLD}{char * width}{Colors.RESET}")
    print(f"{Colors.CYAN}{Colors.BOLD}{title.center(width)}{Colors.RESET}")
    print(f"{Colors.CYAN}{Colors.BOLD}{char * width}{Colors.RESET}\n")

def print_section(title):
    """Print a section header"""
    print(f"\n{Colors.YELLOW}{Colors.BOLD}▶ {title}{Colors.RESET}")
    print(f"{Colors.YELLOW}{'─' * (len(title) + 2)}{Colors.RESET}")

def print_success(message):
    """Print a success message"""
    print(f"{Colors.GREEN}✓ {message}{Colors.RESET}")

def print_error(message):
    """Print an error message"""
    print(f"{Colors.RED}✗ {message}{Colors.RESET}")

def print_warning(message):
    """Print a warning message"""
    print(f"{Colors.YELLOW}⚠ {message}{Colors.RESET}")

def print_info(message):
    """Print an info message"""
    print(f"{Colors.BLUE}ℹ {message}{Colors.RESET}")

def print_step(step_num, total_steps, message):
    """Print a step with progress indicator"""
    progress = f"[{step_num}/{total_steps}]"
    print(f"{Colors.MAGENTA}{Colors.BOLD}{progress}{Colors.RESET} {message}")

def show_spinner(duration=1):
    """Show a simple spinner for visual feedback"""
    spinner_chars = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
    for i in range(duration * 10):
        char = spinner_chars[i % len(spinner_chars)]
        print(f"\r{Colors.CYAN}{char}{Colors.RESET} Processing...", end='', flush=True)
        time.sleep(0.1)
    print("\r" + " " * 20 + "\r", end='')  # Clear the spinner

def parse_arguments():
    """Parse command line arguments"""
    parser = argparse.ArgumentParser(
        description="Maintenance update script to automate common tasks for maintenance updates across different versions of OCP",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python maintenance-update.py                           # Run without release version
  python maintenance-update.py --release release-4.19    # Run with release flag
  python maintenance-update.py --release release-4.19 --version-deps github.com/openshift/api github.com/custom/dependency    # Run with custom version dependencies
  python maintenance-update.py --latest-deps github.com/openshift-online/ocm-sdk-go github.com/custom/latest-dependency       # Run with custom latest dependencies
        """
    )
    
    parser.add_argument(
        '--release',
        type=str,
        help='OpenShift release version (e.g., release-4.19, release-4.20)'
    )
    
    parser.add_argument(
        '--version-deps',
        nargs='*',
        default=['github.com/openshift/api', 'github.com/openshift/cluster-version-operator'],
        help='List of dependencies to update with release version (default: empty)'
    )
    
    parser.add_argument(
        '--latest-deps',
        nargs='*',
        default=['github.com/openshift-online/ocm-sdk-go', 'github.com/openshift/osde2e-common'],
        help='List of dependencies to update to latest version (default: empty)'
    )
    
    return parser.parse_args()

def run_command(command, capture_output=False, check=True, show_command=True):
    """Run a shell command and handle errors"""
    cmd_str = ' '.join(command) if isinstance(command, list) else command
    
    if show_command:
        print(f"  {Colors.BLUE}${Colors.RESET} {cmd_str}")
    
    try:
        if capture_output:
            result = subprocess.run(command, shell=isinstance(command, str), 
                                  capture_output=True, text=True, check=check)
            return result.stdout.strip()
        else:
            result = subprocess.run(command, shell=isinstance(command, str), check=check)
            if result.returncode == 0 and show_command:
                print_success(f"Command completed successfully")
            return result.returncode == 0
    except subprocess.CalledProcessError as e:
        print_error(f"Command failed: {e}")
        if check:
            sys.exit(1)
        return "" if capture_output else False

def get_repo_root():
    """Get the repository root directory"""
    try:
        return run_command(['git', 'rev-parse', '--show-toplevel'], capture_output=True)
    except Exception as e:
        logger.error(f"Failed to get repository root: {e}")
        sys.exit(1)

def validate_release_version(release_version):
    """Validate the release version format"""
    if not release_version.startswith('release-'):
        logger.error("Release version must start with 'release-' (e.g., release-4.19)")
        sys.exit(1)
    
    # Extract version number for validation
    version_part = release_version.replace('release-', '')
    try:
        float(version_part)
    except ValueError:
        logger.error(f"Invalid release version format: {release_version}")
        sys.exit(1)

def update_version_based_dependencies(release_version, version_deps):
    """Update version-based dependencies"""
    print_section(f"Updating Version Dependencies ({release_version})")
    
    # Filter out empty strings and update version based dependencies
    filtered_deps = [dep for dep in version_deps if dep.strip()]
    if not filtered_deps:
        print_info("No version dependencies specified, skipping version-based updates")
        return
    
    print_info(f"Found {len(filtered_deps)} dependencies to update:")
    for i, dep in enumerate(filtered_deps, 1):
        print(f"  {Colors.CYAN}{i}.{Colors.RESET} {dep}")
    
    print()
    dependencies = [f"{dep}@{release_version}" for dep in filtered_deps]
    
    for i, dep in enumerate(dependencies, 1):
        print_step(i, len(dependencies), f"Updating {dep}")
        run_command(['go', 'get', dep])
    
    print_success(f"Updated {len(dependencies)} version-based dependencies")
    
    # Update dependencies for controller-runtime based on the versions
    print_section("Updating Controller Runtime")
    controller_runtime_versions = {
        'release-4.19': 'v0.20.0',
        'release-4.20': 'v0.21.0'
    }
    
    if release_version in controller_runtime_versions:
        controller_runtime_version = controller_runtime_versions[release_version]
        dep = f"sigs.k8s.io/controller-runtime@{controller_runtime_version}"
        print_step(1, 1, f"Updating {dep}")
        run_command(['go', 'get', dep])
        print_success("Controller runtime updated successfully")
    else:
        print_error(f"Unrecognized release version: {release_version}")
        print_info(f"Supported versions: {', '.join(controller_runtime_versions.keys())}")
        sys.exit(1)

def update_latest_dependencies(latest_deps):
    """Update latest dependencies"""
    print_section("Updating Latest Dependencies")
    
    if not latest_deps or (len(latest_deps) == 1 and not latest_deps[0].strip()):
        print_info("No latest dependencies specified, skipping latest updates")
        return
    
    print_info(f"Found {len(latest_deps)} dependencies to update:")
    for i, dep in enumerate(latest_deps, 1):
        print(f"  {Colors.CYAN}{i}.{Colors.RESET} {dep}")
    
    print()
    for i, dep in enumerate(latest_deps, 1):
        print_step(i, len(latest_deps), f"Updating {dep}")
        run_command(['go', 'get', dep])
    
    print_success(f"Updated {len(latest_deps)} latest dependencies")

def has_git_changes():
    """Check if there are any git changes"""
    try:
        output = run_command(['git', 'status', '--porcelain'], capture_output=True)
        if isinstance(output, str):
            return len(output) > 0
        else:
            return False
    except Exception:
        return False

def show_git_diff():
    """Show git diff information"""
    print_section("Git Changes Summary")
    
    print_info("Current changes:")
    run_command(['git', '--no-pager', 'diff'], show_command=False)
    
    print(f"\n{Colors.YELLOW}Status:{Colors.RESET}")
    run_command(['git', 'status', '--porcelain'], show_command=False)
    
    print(f"\n{Colors.YELLOW}Statistics:{Colors.RESET}")
    run_command(['git', '--no-pager', 'diff', '--stat'], show_command=False)

def prompt_for_commit(repo_root, commit_message):
    """Prompt user for committing changes"""
    try:
        print(f"\n{Colors.YELLOW}📝 Commit Changes{Colors.RESET}")
        print(f"Default message: {Colors.CYAN}\"{commit_message}\"{Colors.RESET}")
        confirm = input(f"\n{Colors.BOLD}Do you wish to commit the changes? (y/yes/n/No):{Colors.RESET} ").strip().lower()
        
        if confirm in ['y', 'yes']:
            # Ask for custom commit message
            print(f"\n{Colors.YELLOW}✏️  Custom Commit Message{Colors.RESET}")
            print(f"Press Enter to use default message, or type your custom message:")
            
            custom_message = input(f"{Colors.BOLD}Commit message:{Colors.RESET} ").strip()
            
            # Use custom message if provided, otherwise use default
            final_message = custom_message if custom_message else commit_message
            
            print_info(f"Committing with message: \"{final_message}\"")
            run_command(['git', 'commit', '-m', final_message])
            print_success("Changes committed successfully!")
        else:
            print_warning("Not committing the changes to repository. Exiting.")
            sys.exit(1)
    except KeyboardInterrupt:
        print_error("\nOperation cancelled by user.")
        sys.exit(1)

def prompt_for_add(repo_root):
    """Prompt user for adding changes to git cache"""
    print_section("Git Staging Overview")
    
    # Check for already staged files
    try:
        staged_result = subprocess.run(['git', 'diff', '--cached', '--name-only'], 
                                     capture_output=True, text=True, check=True)
        staged_files = staged_result.stdout.strip()
    except Exception as e:
        print_error(f"Failed to get staged files: {e}")
        sys.exit(1)
    
    # Display already staged files
    if staged_files:
        staged_list = [f for f in staged_files.split('\n') if f.strip()]
        print_success(f"Already staged ({len(staged_list)} files):")
        for i, file in enumerate(staged_list, 1):
            print(f"  {Colors.GREEN}{i}.{Colors.RESET} {file}")
        print()
    else:
        print_info("No files currently staged")
    
    # Check for unstaged changes
    try:
        unstaged_result = subprocess.run(['git', 'diff', '--name-only'], 
                                       capture_output=True, text=True, check=True)
        unstaged_files = unstaged_result.stdout.strip()
    except Exception as e:
        print_error(f"Failed to get unstaged files: {e}")
        sys.exit(1)

    if not unstaged_files:
        if staged_files:
            print_info("All changes are already staged - ready to commit!")
        else:
            print_info("No unstaged files to add")
        return
    
    # Process unstaged files
    files = [f for f in unstaged_files.split('\n') if f.strip()]
    print_warning(f"Unstaged changes ({len(files)} files):")
    for i, file in enumerate(files, 1):
        print(f"  {Colors.YELLOW}{i}.{Colors.RESET} {file}")
    
    print(f"\n{Colors.BOLD}Select files to stage:{Colors.RESET}")
    
    try:
        for i, file in enumerate(files, 1):
            print(f"\n{Colors.CYAN}[{i}/{len(files)}]{Colors.RESET} {file}")
            confirm = input(f"{Colors.BOLD}Add this file to git cache? (y/yes/n/No/a/all):{Colors.RESET} ").strip().lower()
            
            if confirm in ['y', 'yes']:
                print_success(f"Adding {file} to git cache")
                run_command(['git', 'add', file])
            elif confirm in ['a', 'all']:
                print_success("Adding all remaining files to git cache")
                remaining_files = files[i-1:]  # Include current file and all remaining
                for remaining_file in remaining_files:
                    run_command(['git', 'add', remaining_file], show_command=False)
                print_info(f"Added {len(remaining_files)} files to git cache")
                break
            else:
                print_warning(f"Skipping {file}")
    except KeyboardInterrupt:
        print_error("\nOperation cancelled by user.")
        sys.exit(1)

def handle_git_changes(repo_root, commit_message, should_commit=True):
    """
    Handle git changes - show diff and optionally prompt for commit
    
    Args:
        repo_root (str): Repository root path
        commit_message (str): Default commit message to use
        should_commit (bool): Whether to prompt for commit (True) or just stage files (False)
    
    Examples:
        # Full commit workflow
        handle_git_changes(repo_root, "Update dependencies", should_commit=True)
        
        # Stage only, no commit
        handle_git_changes(repo_root, "WIP: testing changes", should_commit=False)
    """
    if not has_git_changes():
        print_success("No git changes detected - repository is clean")
        return
    
    print_warning("Found local changes in the git repository")
    
    show_git_diff()
    prompt_for_add(repo_root)
    
    if should_commit:
        prompt_for_commit(repo_root, commit_message)
    else:
        print_info("Skipping commit as requested - files have been staged only")
        print_warning("Remember to commit your changes manually when ready")

def update_boilerplate(repo_root):
    """Update boilerplate"""
    print_section("Updating Boilerplate")
    print_info("Running boilerplate update...")
    
    run_command(['make', 'boilerplate-update'])
    print_success("Boilerplate updated successfully")

def run_make_tests(repo_root, validate_message):
    """Run validation tests using container-make script"""
    print_section(validate_message)
    print_info("Running validation tests with container-make...")

    container_make_path = os.path.join(repo_root, 'boilerplate', '_lib', 'container-make')

    print_info(f"Using container-make script: {container_make_path}")
    run_command([container_make_path])
    
    print_success("Validation tests completed successfully")

def main():
    """Main function"""
    start_time = datetime.now()
    
    # Configure logging with colors
    logging.basicConfig(
        level=logging.INFO, 
        format=f'{Colors.BLUE}%(asctime)s{Colors.RESET} - %(levelname)s - %(message)s',
        datefmt='%H:%M:%S'
    )
    
    # Welcome header
    print_header("🔧 MAINTENANCE UPDATE SCRIPT 🔧")
    print_info(f"Started at {start_time.strftime('%Y-%m-%d %H:%M:%S')}")
    
    args = parse_arguments()
    
    # Determine release version from arguments
    release_version = args.release

    # Show configuration
    print_section("Configuration")
    print(f"  {Colors.CYAN}Release version:{Colors.RESET} {release_version or 'None'}")
    print(f"  {Colors.CYAN}Version dependencies:{Colors.RESET} {len(args.version_deps)} items")
    print(f"  {Colors.CYAN}Latest dependencies:{Colors.RESET} {len(args.latest_deps)} items")

    # Get repository root
    repo_root = get_repo_root()
    print_success(f"Repository root: {repo_root}")
    
    # Change to repository root directory
    os.chdir(repo_root)
    
    # Begin updating the dependencies
    print_section("Preparing Dependencies")
    
    # Update version based dependencies if release version is provided
    if release_version:
        validate_release_version(release_version)
        update_version_based_dependencies(release_version, args.version_deps)

    # Update latest dependencies
    print_step(1, 1, "Updating latest dependencies...")
    update_latest_dependencies(args.latest_deps)
 
    # Tidy up dependencies before validations are done
    print_step(2, 1, "Running go mod tidy...")
    run_command(['go', 'mod', 'tidy'])
    print_success("Dependencies tidied")

    # Run make tests to validate go.mod update changes
    run_make_tests(repo_root, "Validate go.mod update changes")

    # Handle git changes for dependency updates
    # Use should_commit=True to commit immediately, or should_commit=False to stage only
    handle_git_changes(repo_root, commit_message="Updating go.mod dependencies", should_commit=True)
    
    # Update boilerplate
    update_boilerplate(repo_root)
    
    # Run make tests to validate boilerplate update changes
    run_make_tests(repo_root, "Validate boilerplate update changes")

    # Handle git changes after validation is done for boilerplate
    handle_git_changes(repo_root, commit_message="Updating boilerplate dependencies", should_commit=True)

    # Summary
    end_time = datetime.now()
    duration = end_time - start_time
    
    print_header("🎉 MAINTENANCE UPDATE COMPLETED 🎉", char='*')
    print_success(f"Completed at {end_time.strftime('%Y-%m-%d %H:%M:%S')}")
    print_info(f"Total duration: {duration.total_seconds():.1f} seconds")
    print(f"\n{Colors.GREEN}{Colors.BOLD}All maintenance tasks completed successfully!{Colors.RESET}\n")

if __name__ == "__main__":
    main()

