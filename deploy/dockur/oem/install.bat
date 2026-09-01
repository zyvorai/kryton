@echo off
setlocal EnableExtensions
echo [%DATE% %TIME%] Kryton golden image OEM script >> C:\OEM\install.log

REM VirtIO block/network drivers are injected by dockur/windows during setup.
REM Install QEMU Guest Agent when an MSI is bundled under C:\OEM\qemu-ga.
if exist C:\OEM\qemu-ga\qemu-ga-x64.msi (
  echo Installing QEMU Guest Agent... >> C:\OEM\install.log
  msiexec /i C:\OEM\qemu-ga\qemu-ga-x64.msi /qn /norestart
)

REM Generalize the image for KubeVirt / CDI cloning (dockur FAQ + Kryton golden pipeline).
echo Running Sysprep... >> C:\OEM\install.log
C:\Windows\System32\Sysprep\Sysprep.exe /generalize /oobe /shutdown /quiet
