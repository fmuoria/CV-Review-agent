# Windows Installer

This directory contains the Inno Setup script for creating a Windows installer for CV Review Agent.

## Prerequisites

1. **Inno Setup 6** or later installed on Windows
   - Download from: https://jrsoftware.org/isinfo.php

2. **Built executable**
   - Run `scripts/build_windows.bat` first to build the GUI executable

## Building the Installer

### Option 1: Using Inno Setup GUI
1. Open Inno Setup Compiler
2. Open the file `setup.iss`
3. Click "Build" → "Compile"
4. The installer will be created in `installer/windows/output/`

### Option 2: Using Command Line
```batch
cd installer\windows
"C:\Program Files (x86)\Inno Setup 6\ISCC.exe" setup.iss
```

## Output

The installer will be created as:
- `installer/windows/output/CVReviewAgent_Setup_1.0.0.exe`

## Installer Features

- Creates desktop shortcut (optional)
- Creates Start Menu entry
- Installs to Program Files
- Creates config directory in `%APPDATA%\CVReviewAgent`
- Uninstaller included

## Icon

To use a custom icon:
1. Create or obtain a `.ico` file with your application icon
2. Place it in this directory as `icon.ico`
3. Rebuild the installer

If no icon is provided, the default icon will be used.

## Post-Installation

After installation, users will need to:
1. Configure Google Cloud credentials in Settings
2. Configure Gmail OAuth credentials (credentials.json)
3. Authenticate with Gmail on first use

## Distribution

The generated installer is a single executable that can be distributed to end users. No additional dependencies are required.
