@echo off
echo Building DJI RC Nx Translator...

:: Ensure goversioninfo is installed
where goversioninfo >nul 2>&1
if %ERRORLEVEL% NEQ 0 (
    echo Installing goversioninfo tool...
    go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
)

:: Generate resources
echo Generating resources...
go generate ./...

:: Build for current architecture with debugging information
echo Building application...
go build -v -ldflags="-H=windowsgui" -o DJI_RC_Nx_Translator.exe

:: Build was successful
IF %ERRORLEVEL% EQU 0 (
    echo Build completed successfully.
    echo Executable: DJI_RC_Nx_Translator.exe
    
    :: Run the program if requested
    if "%1"=="run" (
        echo Running application...
        start DJI_RC_Nx_Translator.exe
    )
) ELSE (
    echo Build failed with error code %ERRORLEVEL%
    exit /b %ERRORLEVEL%
)