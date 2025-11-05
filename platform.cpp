/* DUKTO - A simple, fast and multi-platform file transfer tool for LAN users
 * Copyright (C) 2011 Emanuele Colombo
 *
 * This program is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 2 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 59 Temple Place - Suite 330, Boston, MA 02111-1307, USA.
 */

#include "platform.h"

#include <QString>
#include <QHostInfo>
#include <QFile>
#include <QDir>
#include <QRegularExpression>
#include <QStandardPaths>
#include <QImage>
#include <QProcessEnvironment>
#include <QGuiApplication>
#include <QStyleHints>
#include <QPalette>
#include "settings.h"

#if defined(Q_OS_MAC)
#include <QTemporaryFile>
#include <CoreServices/CoreServices.h>
#endif

#if defined(Q_OS_WIN)
#include <windows.h>
#include <lmaccess.h>
#include <QLibrary>

// undocumented, starting from Win10 1809, replaced from 20H2
#define DWMWA_USE_IMMERSIVE_DARK_MODE_OLD 19
// starting from Win10 20H2
#ifndef DWMWA_USE_IMMERSIVE_DARK_MODE
#define DWMWA_USE_IMMERSIVE_DARK_MODE 20
#endif
#endif

#if defined(Q_OS_LINUX) && !defined(Q_OS_ANDROID)
#include <QtDBus/QDBusInterface>
#include <QUrl>
#include <unistd.h>
#include <sys/types.h>
#endif

#if defined(Q_OS_ANDROID)
#include "androidutils.h"
#endif

#if defined(GSETTINGS_SUPPORT)
#include <qgsettings.h>
#endif

// Returns the buddy name
QString Platform::getUsername()
{
    QString buddyName = gSettings->buddyName();
    if (buddyName.isEmpty() == false) {
        return buddyName;
    }
    return getSystemUsername();
}

// Returns the system username
QString Platform::getSystemUsername()
{
    // Save in a static variable so that It's always ready
    static QString username;
    if (!username.isEmpty()) return username;

#ifdef Q_OS_ANDROID
    username = "User";
#else
    // Looking for the username
    QString uname(env("USERNAME"));
    if (uname.isEmpty())
    {
        uname = env("USER");
        if (uname.isEmpty())
            uname = "Unknown";
    }
    username = uname.replace(0, 1, uname.at(0).toUpper());
#endif
    return username;
}

// Returns the hostname
QString Platform::getHostname()
{
    // Save in a static variable so that It's always ready
    static QString hostname;
    if (!hostname.isEmpty()) return hostname;

#ifdef Q_OS_ANDROID
    hostname = AndroidSettings::getStringValue(AndroidSettings::Global, "device_name");
    if (hostname.isEmpty()) {
        hostname = AndroidEnvironment::buildInfo("MODEL");
    }
    hostname.replace(QChar(' '), QChar('-'));
#else
    // Get the hostname
    // (replace ".local" for MacOSX)
    hostname = QHostInfo::localHostName().replace(".local", "");
#endif
    return hostname;
}

// Returns the platform name
QString Platform::getPlatformName()
{
#if defined(Q_OS_WIN)
    return "Windows";
#elif defined(Q_OS_MAC)
    return "Macintosh";
#elif defined(Q_OS_ANDROID)
    return "Android";
#elif defined(Q_OS_LINUX)
    return "Linux";
#else
    return "Unknown";
#endif
}

// Returns the platform avatar path
QString Platform::getAvatarPath()
{
    // user specified avatar
#if QT_VERSION >= QT_VERSION_CHECK(5, 4, 0)
    QDir dir(QStandardPaths::writableLocation(QStandardPaths::AppLocalDataLocation));
#else
    QDir dir(QStandardPaths::writableLocation(QStandardPaths::DataLocation));
#endif
    QString avatarFile = dir.filePath("avatar.png");
    QImage image(avatarFile);
    if (image.isNull() == false) {
        return avatarFile;
    }

    // default avatar
#if defined(Q_OS_WIN)
    QString username = getSystemUsername().replace("\\", "+");
    QString path = env("LOCALAPPDATA") + "\\Temp\\" + username + ".bmp";
    if (!QFile::exists(path))
        path = getWinTempAvatarPath();
    if (!QFile::exists(path))
        path = env("PROGRAMDATA") + "\\Microsoft\\User Account Pictures\\Guest.bmp";
    if (!QFile::exists(path))
        path = env("ALLUSERSPROFILE") + "\\" + QDir(env("APPDATA")).dirName() + "\\Microsoft\\User Account Pictures\\" + username + ".bmp";
    if (!QFile::exists(path))
        path = env("ALLUSERSPROFILE") + "\\" + QDir(env("APPDATA")).dirName() + "\\Microsoft\\User Account Pictures\\Guest.bmp";
    return path;
#elif defined(Q_OS_MAC)
    return getMacTempAvatarPath();
#elif defined(Q_OS_ANDROID)
    return "";
#elif defined(Q_OS_LINUX)
    return getLinuxAvatarPath();
#else
    return "";
#endif
}

// Returns the platform default output path
QString Platform::getDefaultPath()
{
    // For Windows it's the Desktop folder
#if defined(Q_OS_WIN)
    return env("USERPROFILE") + "\\Desktop";
#elif defined(Q_OS_MAC)
    return env("HOME") + "/Desktop";
#elif defined(Q_OS_ANDROID)
    return "";
#elif defined(Q_OS_UNIX)
    return env("HOME");
#else
    #error "Unknown default path for this platform"
#endif

}

#if defined(Q_OS_LINUX) && !defined(Q_OS_ANDROID)
// Special function for Linux
QString Platform::getLinuxAvatarPath()
{
    QString path;

    // Gnome2 / Xfce / KDE5 check
    path = env("HOME") + "/.face";
    if (QFile::exists(path)) return path;

    // KDE5 check
    path.append(".icon");
    if (QFile::exists(path)) return path;

    QString uid = QString::number(getuid());
    auto getIconFromDbus = [&uid](const QString &service)->QString {
        QDBusInterface iface(service, "/" + QString(service).replace(QChar('.'), QChar('/')) + "/User" + uid, service + ".User", QDBusConnection::systemBus());
        if (iface.isValid()) {
            QVariant iconFile = iface.property("IconFile");
            if (iconFile.isValid()) {
                QString path = iconFile.toString();
                if (path.startsWith("file://")) {
                    path = QUrl(path).toLocalFile();
                }
                if (QFile::exists(path)) {
                    return path;
                }
            }
        }
        return QString();
    };

    // Deepin check
    path = getIconFromDbus("com.deepin.daemon.Accounts");
    if (!path.isEmpty()) {
        return path;
    }

    // Gnome3 check
    path = getIconFromDbus("org.freedesktop.Accounts");
    if (!path.isEmpty()) {
        return path;
    }

    // Not found
    return "";
}
#endif

#if defined(Q_OS_MAC)
static QTemporaryFile macAvatar;

// Special function for OSX
QString Platform::getMacTempAvatarPath()
{
    // Get image data from system
    QByteArray qdata;
    CSIdentityQueryRef query = CSIdentityQueryCreateForCurrentUser(kCFAllocatorSystemDefault);
    CFErrorRef error;
	if (CSIdentityQueryExecute(query, kCSIdentityQueryGenerateUpdateEvents, &error)) {
        CFArrayRef foundIds = CSIdentityQueryCopyResults(query);
        if (CFArrayGetCount(foundIds) == 1) {
            CSIdentityRef userId = (CSIdentityRef) CFArrayGetValueAtIndex(foundIds, 0);
            CFDataRef data = CSIdentityGetImageData(userId);
            //qDebug() << CFDataGetLength(data);
            qdata.resize(CFDataGetLength(data));
            CFDataGetBytes(data, CFRangeMake(0, CFDataGetLength(data)), reinterpret_cast<uint8*>(qdata.data()));
        }
    }
    CFRelease(query);

    // Save it to a temporary file
    macAvatar.open();
    macAvatar.write(qdata);
    macAvatar.close();
    return macAvatar.fileName();
}
#endif

#if defined(Q_OS_WIN)

#include <objbase.h>

#ifndef ARRAYSIZE
#define ARRAYSIZE(a) \
  ((sizeof(a) / sizeof(*(a))) / \
  static_cast<size_t>(!(sizeof(a) % sizeof(*(a)))))
#endif

typedef HRESULT (WINAPI*pfnSHGetUserPicturePathEx)(
    LPCWSTR pwszUserOrPicName,
    DWORD sguppFlags,
    LPCWSTR pwszDesiredSrcExt,
    LPWSTR pwszPicPath,
    UINT picPathLen,
    LPWSTR pwszSrcPath,
    UINT srcLen
);

// Special function for Windows 8
QString Platform::getWinTempAvatarPath()
{
    // Get file path
    CoInitialize(nullptr);
    HMODULE hMod = LoadLibrary(L"shell32.dll");
    pfnSHGetUserPicturePathEx picPathFn = (pfnSHGetUserPicturePathEx)GetProcAddress(hMod, (LPCSTR)810);
    WCHAR picPath[500] = {0}, srcPath[500] = {0};
    HRESULT ret = picPathFn(nullptr, 0, nullptr, picPath, ARRAYSIZE(picPath), srcPath, ARRAYSIZE(srcPath));
    FreeLibrary(hMod);
    CoUninitialize();
    if (ret != S_OK) return "C:\\missing.bmp";
    QString result = QString::fromWCharArray(picPath, -1);
    return result;
}

#endif

#if defined(Q_OS_WIN)
int Platform::isWinDarkTheme() {
    DWORD buffer;
    DWORD cbData = sizeof(buffer);
    LSTATUS res = RegGetValueW(HKEY_CURRENT_USER, L"Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize", L"AppsUseLightTheme", RRF_RT_REG_DWORD, NULL, &buffer, &cbData);
    if (res == ERROR_SUCCESS) {
        return (buffer == 1 ? 0 : 1);
    }
    return -1;
}
#endif


#if defined(Q_OS_LINUX) && defined(GSETTINGS_SUPPORT)
int Platform::isGSettingsDarkTheme(QGSettings *gs) {
    QString gtkColorScheme = gs->get("color-scheme").toString();
    if (gtkColorScheme == "prefer-dark") {
        return 1;
    } else if (gtkColorScheme == "prefer-light") {
        return 0;
    } else {
        // default
        QString gtkTheme = gs->get("gtk-theme").toString();
        if (gtkTheme.endsWith("-dark"), Qt::CaseInsensitive) {
            return 1;
        }
    }
    return -1;
}
#endif

#if !defined(Q_OS_ANDROID)

QString Platform::env(const QString &name) {
    static const QProcessEnvironment env = QProcessEnvironment::systemEnvironment();
    return env.value(name);
}

#endif

void Platform::setNonClientAreaMode(QWindow *win, bool darkMode) {
#if defined(Q_OS_WIN)
    QLibrary lib("dwmapi.dll");
    if (lib.load()) {
        typedef HRESULT (WINAPI *DwmSetWindowAttribute)(HWND,DWORD,LPCVOID,DWORD); // WinVista+
        DwmSetWindowAttribute pDwmSetWindowAttribute = reinterpret_cast<DwmSetWindowAttribute>(lib.resolve("DwmSetWindowAttribute"));
        if (pDwmSetWindowAttribute != nullptr) {
            HWND hWnd = reinterpret_cast<HWND>(win->winId());
            BOOL value = darkMode ? TRUE : FALSE;
            pDwmSetWindowAttribute(hWnd, DWMWA_USE_IMMERSIVE_DARK_MODE_OLD, &value, sizeof(value));
            pDwmSetWindowAttribute(hWnd, DWMWA_USE_IMMERSIVE_DARK_MODE, &value, sizeof(value));
            // force redraw
            RECT rect;
            GetWindowRect(hWnd, &rect);
            SetWindowPos(hWnd, 0, 0, 0, rect.right - rect.left, rect.bottom - rect.top + 1, SWP_NOREDRAW|SWP_NOACTIVATE|SWP_NOMOVE|SWP_NOZORDER);
            SetWindowPos(hWnd, 0, 0, 0, rect.right - rect.left, rect.bottom - rect.top, SWP_DRAWFRAME|SWP_NOACTIVATE|SWP_NOMOVE|SWP_NOZORDER);
        }
    }
#elif defined(Q_OS_ANDROID)
    Q_UNUSED(win)
    AndroidTheme::setAppNightMode(darkMode);
#else
    Q_UNUSED(win)
    Q_UNUSED(darkMode)
#endif
}

bool Platform::isDarkTheme() {
#if 0 && QT_VERSION >= QT_VERSION_CHECK(6, 5, 0)
    const QStyleHints *hints = QGuiApplication::styleHints();
    const Qt::ColorScheme qtColorScheme = hints->colorScheme();
    if (qtColorScheme == Qt::ColorScheme::Dark) {
        return true;
    } else if (qtColorScheme == Qt::ColorScheme::Light) {
        return false;
    }
#endif
#if defined(Q_OS_WIN)
    int r = isWinDarkTheme();
    if (r != -1) {
        return r == 1;
    }
#elif defined(Q_OS_ANDROID)
    return AndroidTheme::isNightMode();
#elif defined(Q_OS_LINUX)
#ifdef GSETTINGS_SUPPORT
    const QByteArray schema = "org.gnome.desktop.interface";
    if (QGSettings::isSchemaInstalled(schema)) {
        QGSettings gsettings(schema);
        int r = isGSettingsDarkTheme(&gsettings);
        if (r >= 0) {
            return r == 1;
        }
    }
#endif
#endif
    return false;
}

PlatformObserver::PlatformObserver(QObject *parent) : QObject(parent) {
    observe();
}

PlatformObserver::~PlatformObserver() {
#if QT_VERSION < QT_VERSION_CHECK(6, 5, 0) && defined(Q_OS_LINUX) && defined(GSETTINGS_SUPPORT)
    delete gsettings;
#endif
}

void PlatformObserver::observe() {
#if QT_VERSION >= QT_VERSION_CHECK(6, 5, 0)
    const QStyleHints *hints = QGuiApplication::styleHints();
    connect(hints, &QStyleHints::colorSchemeChanged, this, [this](Qt::ColorScheme cs) {
        emit colorSchemeChanged(cs == Qt::ColorScheme::Dark);
    });
#else

#ifdef Q_OS_LINUX
#ifdef GSETTINGS_SUPPORT
    const QByteArray schema = "org.gnome.desktop.interface";
    if (QGSettings::isSchemaInstalled(schema)) {
        gsettings = new QGSettings(schema);
        connect(gsettings, &QGSettings::changed, this, &PlatformObserver::gsettingsChanged);
    }
#endif
#endif

#endif
}


#if QT_VERSION < QT_VERSION_CHECK(6, 5, 0)

#ifdef Q_OS_WIN
bool PlatformObserver::winEvent(MSG *message, void *result) {
    Q_UNUSED(result);
    if (message->message == WM_SETTINGCHANGE) {
        LPTSTR str = reinterpret_cast<LPTSTR>(message->lParam);
        if (lstrcmp(str, L"ImmersiveColorSet") == 0) {
            emit colorSchemeChanged(Platform::isWinDarkTheme());
        }
    }
    return false;
}
#endif

#if defined(Q_OS_LINUX) && defined(GSETTINGS_SUPPORT)

void PlatformObserver::gsettingsChanged(const QString &key) {
    if (key == "color-scheme" || (key == "gtk-theme" && gsettings->get("color-scheme").toString() == "default")) {
        int r = Platform::isGSettingsDarkTheme(gsettings);
        if (r >= 0) {
            emit colorSchemeChanged(r == 1);
        }
    }
}

#endif

#endif
