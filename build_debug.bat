@echo off
echo Building DJI RC Nx Translator (Debug Version)...

:: Make sure goversioninfo is installed
where goversioninfo >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo Installing goversioninfo tool...
    go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
)

:: Generate resources
echo Generating resources...
go generate ./...

:: Build debug version (with console window)
echo Building debug version...
go build -v -o DJI_RC_Nx_Translator_Debug.exe

echo Debug build completed.
echo Executable: DJI_RC_Nx_Translator_Debug.exe

:: Run the debug version
DJI_RC_Nx_Translator_Debug.exe