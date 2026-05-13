# WinGet Submission Process - Step by Step

## **STEP 1: Create GitHub Release (Do This First!)**

1. Open: https://github.com/PuemMTH/sir-go/releases
2. Click **"Create a new release"**
3. Fill in these fields:

   **Tag version:** `v5.1.0`
   **Release title:** `SIR v5.1.0`
   **Description:** Copy this text:
   ```
   ## 🎉 What's New

   ### Features
   - Docker Compose scanner with TUI support
   - Live monitoring with auto-refresh
   - Fuzzy search in TUI mode
   - Technical columns: image names and exposed ports
   - Configurable scan depth

   ### Installation
   - **Windows (WinGet):** `winget install PuemMTH.SIR`
   - **macOS:** Download from releases
   - **Linux:** Download from releases

   ### System Requirements
   - Windows 10+, macOS, or Linux
   - Docker daemon running

   [See full changelog](https://github.com/PuemMTH/sir-go/blob/main/CHANGELOG.md)
   ```

4. **Upload your assets:** Click "Attach binaries"
   - Drag/drop all 5 exe files:
     - sir_windows_amd64.exe
     - sir_darwin_amd64
     - sir_darwin_arm64
     - sir_linux_amd64
     - sir_linux_arm64

5. Click **"Publish release"** ✅

---

## **STEP 2: Fork WinGet Repository**

1. Open: https://github.com/microsoft/winget-pkgs
2. Click **"Fork"** (top right)
3. Create fork on your account
4. Clone it locally:
   ```powershell
   git clone https://github.com/YOUR_USERNAME/winget-pkgs.git
   cd winget-pkgs
   ```

---

## **STEP 3: Create Manifest File**

Create this folder structure:
```
manifests/
  p/
    PuemMTH/
      SIR/
        5.1.0/
          PuemMTH.SIR.yaml  ← Copy your manifest here
```

Copy `PuemMTH.SIR.yaml` from your sir-go folder to:
`manifests/p/PuemMTH/SIR/5.1.0/PuemMTH.SIR.yaml`

---

## **STEP 4: Submit Pull Request**

1. In your cloned winget-pkgs folder:
   ```powershell
   git add manifests/p/PuemMTH/SIR/
   git commit -m "Add PuemMTH.SIR v5.1.0"
   git push origin main
   ```

2. Open GitHub: https://github.com/microsoft/winget-pkgs
3. You'll see a **"Create Pull Request"** button
4. Click it and submit
5. Add description:
   ```
   Adds PuemMTH.SIR - Service Inspector Reporter
   A terminal tool for monitoring Docker Compose services
   ```

---

## **STEP 5: Wait for Approval**

- Microsoft bots will validate your manifest (usually instant)
- If there are issues, they'll comment on your PR
- Once approved, it merges automatically
- Users can then: `winget install PuemMTH.SIR` ✅

---

## **Files Ready for You:**
- ✅ `PuemMTH.SIR.yaml` - WinGet manifest
- ✅ `RELEASE_NOTES.md` - Release description
- ✅ `INSTALL.md` - Installation guide
- ✅ Updated `README.md` - WinGet instructions
