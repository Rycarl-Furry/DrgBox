[Version]
Class=IEXPRESS
SEDVersion=3
[Options]
PackagePurpose=InstallApp
ShowInstallProgramWindow=0
HideExtractAnimation=1
UseLongFileName=1
InsideCompressed=1
CAB_FixedSize=0
CAB_ResvCodeSigning=0
RebootMode=N
InstallPrompt=
DisplayLicense=
FinishMessage=DRGBOX 已安装到 D:\Car1N0tCat\DRGBOX，并创建了桌面快捷方式。
TargetName=D:\Car1N0tCat\DrgBoxDesktop\build\installer\DRGBOX-Setup.exe
FriendlyName=DRGBOX 本地分类工具箱
AppLaunched=powershell.exe -NoProfile -ExecutionPolicy Bypass -File install.ps1
PostInstallCmd=<None>
AdminQuietInstCmd=
UserQuietInstCmd=
SourceFiles=SourceFiles
[SourceFiles]
SourceFiles0=D:\Car1N0tCat\DrgBoxDesktop\installer\payload\
[SourceFiles0]
%FILE0%=
%FILE1%=
[Strings]
FILE0=DrgBoxDesktop.exe
FILE1=install.ps1
