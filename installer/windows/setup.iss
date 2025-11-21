; CV Review Agent - Inno Setup Script
; Generates a Windows installer for the CV Review Agent GUI application

#define AppName "CV Review Agent"
#define AppVersion "1.0.0"
#define AppPublisher "CV Review Agent Team"
#define AppURL "https://github.com/fmuoria/CV-Review-agent"
#define AppExeName "cv-review-agent-gui.exe"
#define AppDescription "AI-powered CV Review and Ranking System"

[Setup]
AppId={{A8B9C0D1-E2F3-4A5B-6C7D-8E9F0A1B2C3D}
AppName={#AppName}
AppVersion={#AppVersion}
AppVerName={#AppName} {#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
AppUpdatesURL={#AppURL}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
LicenseFile=..\..\LICENSE
OutputDir=output
OutputBaseFilename=CVReviewAgent_Setup_{#AppVersion}
SetupIconFile=icon.ico
Compression=lzma
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=lowest
ArchitecturesInstallIn64BitMode=x64

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "..\..\cmd\gui\{#AppExeName}"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Comment: "{#AppDescription}"
Name: "{group}\{cm:UninstallProgram,{#AppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExeName}"; Tasks: desktopicon; Comment: "{#AppDescription}"

[Run]
Filename: "{app}\{#AppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(AppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[Code]
// First-run configuration wizard
procedure InitializeWizard();
var
  ConfigPage: TInputQueryWizardPage;
begin
  ConfigPage := CreateInputQueryPage(wpWelcome,
    'Configuration', 'Google Cloud Settings',
    'Please enter your Google Cloud configuration. You can change these later in the application settings.');
  
  ConfigPage.Add('Google Cloud Project ID:', False);
  ConfigPage.Add('Google Cloud Location:', False);
  
  // Set default values
  ConfigPage.Values[0] := '';
  ConfigPage.Values[1] := 'us-central1';
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  ConfigPath: String;
  ConfigFile: String;
  ConfigDir: String;
begin
  if CurStep = ssPostInstall then
  begin
    // Create config directory
    ConfigDir := ExpandConstant('{userappdata}\CVReviewAgent');
    if not DirExists(ConfigDir) then
      CreateDir(ConfigDir);
      
    // Create initial config file if it doesn't exist
    ConfigPath := ConfigDir + '\config.json';
    if not FileExists(ConfigPath) then
    begin
      ConfigFile := '{'#13#10 +
        '  "google_cloud_project": "",'#13#10 +
        '  "google_cloud_location": "us-central1",'#13#10 +
        '  "google_credentials_path": "",'#13#10 +
        '  "gmail_credentials_path": "",'#13#10 +
        '  "uploads_dir": "uploads"'#13#10 +
        '}';
      SaveStringToFile(ConfigPath, ConfigFile, False);
    end;
  end;
end;
