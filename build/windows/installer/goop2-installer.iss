#define MyAppName "Goop2"
#ifndef MyAppVersion
  #define MyAppVersion "dev"
#endif
#define MyAppPublisher "Peter van de Pas"
#define MyAppURL "https://github.com/petervdpas/goop2"
#define MyAppExeName "goop2.exe"

[Setup]
AppId={{7E3F8A2D-B1C4-4D9E-A6F5-2C8E9D1B4A7F}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
; SourceDir = project root (three levels up from this .iss file)
SourceDir=..\..\..
OutputDir=build\windows\installer\output
OutputBaseFilename=goop2-{#MyAppVersion}-windows-amd64-setup
SetupIconFile=build\windows\icon.ico
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=lowest
UninstallDisplayIcon={app}\{#MyAppExeName}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "build\bin\goop2.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "build\windows\icon.ico"; DestDir: "{app}"; DestName: "goop2.ico"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent
