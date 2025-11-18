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
#include <CoreFoundation/CoreFoundation.h>
#endif

#if defined(Q_OS_WIN)
#include <windows.h>
#include <shlobj.h>
#include <lmcons.h> // for UNLEN
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
#include <QtDBus/QDBusReply>
#include <QUrl>
#include <unistd.h>
#include <sys/types.h>

#define DBUS_PORTAL_SERVICE "org.freedesktop.portal.Desktop"
#define DBUS_PORTAL_PATH "/org/freedesktop/portal/desktop"
#define DBUS_PORTAL_SETTINGS_INTERFACE "org.freedesktop.portal.Settings"
#define PORTAL_SETTINGS_NAMESPACE "org.freedesktop.appearance"
#define COLOR_SCHEME_KEY "color-scheme"

#endif

#if defined(Q_OS_ANDROID)
#include "androidutils.h"
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
QString Platform::getSystemUsername() {
    // Save in a static variable so that It's always ready
    static QString username;
    if (!username.isEmpty()) return username;

#if defined(Q_OS_ANDROID)
    username = "User";
#elif defined(Q_OS_WIN)
    WCHAR buffer[UNLEN + 1] = {0};
    DWORD len = UNLEN + 1;
    if (GetUserName(buffer, &len) != 0) {
        username = QString::fromWCharArray(buffer);
    } else {
        username = env("USERNAME");
    }
#else
    // Looking for the username
    username = env("USER");
#endif
    if (username.isEmpty()) {
        username = "Unknown";
    } else {
        username = username.replace(0, 1, username.at(0).toUpper());
    }
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
    return getWinAvatarPath();
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
    WCHAR dir[1024];
    if (SUCCEEDED(SHGetFolderPath(NULL, CSIDL_DESKTOPDIRECTORY, NULL, 0, dir))) {
        return QString::fromWCharArray(dir);
    } else {
        return env("USERPROFILE") + "\\Desktop";
    }
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
    if (query == NULL) {
        return "";
    }
    if (CSIdentityQueryExecute(query, kCSIdentityQueryGenerateUpdateEvents, NULL)) {
        CFArrayRef foundIds = CSIdentityQueryCopyResults(query);
        if (foundIds != NULL) {
            if (CFArrayGetCount(foundIds) == 1) {
                CSIdentityRef userId = (CSIdentityRef) CFArrayGetValueAtIndex(foundIds, 0);
                CFDataRef data = CSIdentityGetImageData(userId);
                if (data != NULL) {
                    qdata.resize(CFDataGetLength(data));
                    CFDataGetBytes(data, CFRangeMake(0, CFDataGetLength(data)), reinterpret_cast<uint8*>(qdata.data()));
                }
            }
            CFRelease(foundIds);
        }
    }
    CFRelease(query);

    if (qdata.isEmpty()) {
        return "";
    }

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

// Undocumented function for Windows Vista+
// http://undoc.airesoft.co.uk/shell32.dll/SHGetUserPicturePathEx.php
typedef HRESULT (WINAPI*pfnSHGetUserPicturePathEx)(
    LPCWSTR pwszUserOrPicName,
    DWORD sguppFlags,
    LPCWSTR pwszDesiredSrcExt,
    LPWSTR pwszPicPath,
    UINT picPathLen,
    LPWSTR pwszSrcPath,
    UINT srcLen
);

// Undocumented function for Windows XP/2003
// http://undoc.airesoft.co.uk/shell32.dll/SHGetUserPicturePath.php
typedef HRESULT (WINAPI*pfnSHGetUserPicturePathW)(
    LPCWSTR pwszPicOrUserName,
    DWORD sguppFlags,
    LPWSTR pwszPicPath
);

#ifndef LOAD_LIBRARY_SEARCH_SYSTEM32
#define LOAD_LIBRARY_SEARCH_SYSTEM32 0x00000800
#endif

QString Platform::getWinAvatarPath() {
    static const bool isVistaOrLater = ((LOBYTE(LOWORD(GetVersion()))) >= 6);
    CoInitialize(NULL);

    QString path;
    HMODULE hMod = LoadLibraryEx(L"shell32.dll", NULL, LOAD_LIBRARY_SEARCH_SYSTEM32);
    if (hMod != NULL) {
        if (isVistaOrLater) {
            // Vista+
            pfnSHGetUserPicturePathEx SHGetUserPicturePathEx = (pfnSHGetUserPicturePathEx)GetProcAddress(hMod, (LPCSTR)810);
            if (SHGetUserPicturePathEx != nullptr) {
                WCHAR picPath[1024] = {0};
                if (SHGetUserPicturePathEx(NULL, 0, NULL, picPath, ARRAYSIZE(picPath), NULL, 0) == S_OK) {
                    path = QString::fromWCharArray(picPath);
                }
            }
        } else {
            pfnSHGetUserPicturePathW SHGetUserPicturePathW = (pfnSHGetUserPicturePathW)GetProcAddress(hMod, (LPCSTR)233);
            if (SHGetUserPicturePathW != nullptr) {
                WCHAR picPath[1024] = {0};
                if (SHGetUserPicturePathW(NULL, 0, picPath) == S_OK) {
                    path = QString::fromWCharArray(picPath);
                }
            }
        }
        FreeLibrary(hMod);
    }

    if (QFile::exists(path) == false) {
        path.clear();
        QStringList searchPaths;
        QString userPic = getSystemUsername().replace("\\", "+") + ".bmp";
        QString guestPic = "guest.bmp";
        QString dir;

        if (isVistaOrLater) {
            WCHAR picPath[1024];
            if (SUCCEEDED(SHGetFolderPath(NULL, CSIDL_LOCAL_APPDATA, NULL, 0, picPath))) {
                dir = QString::fromWCharArray(picPath) + "\\Temp\\";
            } else {
                dir = env("LOCALAPPDATA") + "\\Temp\\";
            }
            searchPaths << dir + userPic;
            if (SUCCEEDED(SHGetFolderPath(NULL, CSIDL_COMMON_APPDATA, NULL, 0, picPath))) {
                dir = QString::fromWCharArray(picPath) + "\\Microsoft\\User Account Pictures\\";
            } else {
                dir = env("ALLUSERSPROFILE") + "\\Microsoft\\User Account Pictures\\";
            }
            searchPaths << dir + guestPic;
        } else {
            WCHAR picPath[1024];
            if (SUCCEEDED(SHGetFolderPath(NULL, CSIDL_COMMON_APPDATA, NULL, 0, picPath))) {
                dir = QString::fromWCharArray(picPath) + "\\Microsoft\\User Account Pictures\\";
            } else {
                dir = env("ALLUSERSPROFILE") + "\\Application Data\\Microsoft\\User Account Pictures\\";
            }
            searchPaths << dir + userPic;
            searchPaths << dir + guestPic;
        }

        for (const QString &pic : searchPaths) {
            if (QFile::exists(pic)) {
                path = pic;
                break;
            }
        }
    }

    CoUninitialize();
    return path;
}

#endif

#if defined(Q_OS_WIN)
Platform::ThemeScheme Platform::getWinThemeScheme() {
    ThemeScheme scheme = UnknownTheme;
    HKEY hKey;
    if (RegOpenKeyEx(HKEY_CURRENT_USER, L"Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize", 0, KEY_READ, &hKey) == ERROR_SUCCESS) {
        DWORD type, value, cbData = sizeof(value);
        if (RegQueryValueEx(hKey, L"AppsUseLightTheme", NULL, &type, reinterpret_cast<BYTE*>(&value), &cbData) == ERROR_SUCCESS) {
            // other types may also take effect, but we don't care
            if (type == REG_DWORD) {
                scheme = (value == 0 ? DarkTheme : LightTheme);
            }
        }
        RegCloseKey(hKey);
    }
    return scheme;
}
#endif

#if defined(Q_OS_LINUX) && !defined(Q_OS_ANDROID)
Platform::ThemeScheme Platform::getLinuxThemeSchemeFromXdgPortal() {
    // https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.Settings.html
    QDBusInterface iface(DBUS_PORTAL_SERVICE, DBUS_PORTAL_PATH, DBUS_PORTAL_SETTINGS_INTERFACE, QDBusConnection::sessionBus());
    if (iface.isValid() == false) {
        return UnknownTheme;
    }
    bool v1 = false;
    QDBusMessage reply = iface.call("ReadOne", PORTAL_SETTINGS_NAMESPACE, COLOR_SCHEME_KEY);
    if (reply.type() == QDBusMessage::ErrorMessage) {
        QDBusError err(reply);
        if (err.type() != QDBusError::UnknownMethod) {
            return UnknownTheme;
        }
        // Deprecated method
        reply = iface.call("Read", PORTAL_SETTINGS_NAMESPACE, COLOR_SCHEME_KEY);
        if (reply.type() == QDBusMessage::ErrorMessage) {
            return UnknownTheme;
        }
        v1 = true;
    }
    QVariantList args = reply.arguments();
    if (args.isEmpty()) {
        return UnknownTheme;
    }
    QVariant var;
    if (v1) {
        var = reply.arguments().at(0).value<QDBusVariant>().variant().value<QDBusVariant>().variant();
    } else {
        var = reply.arguments().at(0).value<QDBusVariant>().variant();
    }
    bool ok;
    int value = var.toInt(&ok);
    if (ok) {
        // 0: No preference
        // 1: Prefer dark
        // 2: Prefer light
        if (value == 1) {
            return DarkTheme;
        } else if (value == 2) {
            return LightTheme;
        }
    }
    return UnknownTheme;
}
#endif

#if defined(Q_OS_MAC)
Platform::ThemeScheme Platform::getMacThemeScheme() {
    CFStringRef styleKey = CFSTR("AppleInterfaceStyle");
    CFPropertyListRef styleValue = CFPreferencesCopyValue(styleKey, kCFPreferencesAnyApplication, kCFPreferencesCurrentUser, kCFPreferencesAnyHost);
    if (styleValue == NULL) {
        return LightTheme;
    }
    bool isDark = false;
    if (CFGetTypeID(styleValue) == CFStringGetTypeID()) {
        if (CFStringCompare((CFStringRef)styleValue, CFSTR("Dark"), 0) == kCFCompareEqualTo) {
            isDark = true;
        }
    }
    CFRelease(styleValue);
    return isDark ? DarkTheme : UnknownTheme;
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
        typedef HRESULT (WINAPI *fnDwmSetWindowAttribute)(HWND,DWORD,LPCVOID,DWORD); // WinVista+
        fnDwmSetWindowAttribute DwmSetWindowAttribute = reinterpret_cast<fnDwmSetWindowAttribute>(lib.resolve("DwmSetWindowAttribute"));
        if (DwmSetWindowAttribute != nullptr) {
            HWND hWnd = reinterpret_cast<HWND>(win->winId());
            BOOL value = darkMode ? TRUE : FALSE;
            HRESULT ret = DwmSetWindowAttribute(hWnd, DWMWA_USE_IMMERSIVE_DARK_MODE, &value, sizeof(value));
            if (ret != S_OK) {
                ret = DwmSetWindowAttribute(hWnd, DWMWA_USE_IMMERSIVE_DARK_MODE_OLD, &value, sizeof(value));
            }
            if (ret == S_OK) {
                // force redraw
                RECT rect;
                GetWindowRect(hWnd, &rect);
                SetWindowPos(hWnd, 0, 0, 0, rect.right - rect.left, rect.bottom - rect.top + 1, SWP_NOREDRAW|SWP_NOACTIVATE|SWP_NOMOVE|SWP_NOZORDER);
                SetWindowPos(hWnd, 0, 0, 0, rect.right - rect.left, rect.bottom - rect.top, SWP_DRAWFRAME|SWP_NOACTIVATE|SWP_NOMOVE|SWP_NOZORDER);
            }
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
#if QT_VERSION >= QT_VERSION_CHECK(6, 5, 0)
    const QStyleHints *hints = QGuiApplication::styleHints();
    const Qt::ColorScheme qtColorScheme = hints->colorScheme();
    if (qtColorScheme == Qt::ColorScheme::Dark) {
        return true;
    } else if (qtColorScheme == Qt::ColorScheme::Light) {
        return false;
    }
#endif

#if defined(Q_OS_ANDROID)
    return AndroidTheme::isNightMode();
#endif

    ThemeScheme r = UnknownTheme;
#if defined(Q_OS_WIN)
    r = getWinThemeScheme();
#elif defined(Q_OS_LINUX) && !defined(Q_OS_ANDROID)
    r = getLinuxThemeSchemeFromXdgPortal();
#elif defined(Q_OS_MAC)
    r = getMacThemeScheme();
#endif
    if (r != UnknownTheme) {
        return r == DarkTheme;
    }
    return false;
}

#if defined(Q_OS_MAC)
PlatformObserver *obsInst = nullptr;
#endif


PlatformObserver::PlatformObserver(QObject *parent) : QObject(parent) {
    observe();
}

PlatformObserver::~PlatformObserver() {
}

void PlatformObserver::observe() {
#if QT_VERSION >= QT_VERSION_CHECK(6, 5, 0)
    const QStyleHints *hints = QGuiApplication::styleHints();
    connect(hints, &QStyleHints::colorSchemeChanged, this, [this](Qt::ColorScheme cs) {
        emit colorSchemeChanged(cs == Qt::ColorScheme::Dark);
    });
#endif

#if defined(Q_OS_LINUX) && !defined(Q_OS_ANDROID)
    QDBusConnection conn = QDBusConnection::sessionBus();
    if (conn.isConnected()) {
        conn.connect(DBUS_PORTAL_SERVICE, DBUS_PORTAL_PATH, DBUS_PORTAL_SETTINGS_INTERFACE, "SettingChanged", this, SLOT(dbusChanged(QString,QString,QDBusVariant)));
    }
#endif

#if defined(Q_OS_MAC)
    CFNotificationCenterRef nc = CFNotificationCenterGetDistributedCenter();
    CFStringRef notification = CFSTR("AppleInterfaceThemeChangedNotification");
    auto callback = [](CFNotificationCenterRef /*nc*/, void */*observer*/, CFStringRef /*notification*/, const void */*obj*/, CFDictionaryRef /*userInfo*/) {
        Platform::ThemeScheme cs = Platform::getMacThemeScheme();
        emit obsInst->colorSchemeChanged(cs == Platform::DarkTheme);
    };
    obsInst = this;
    CFNotificationCenterAddObserver(nc, NULL, callback, notification, NULL, CFNotificationSuspensionBehaviorDeliverImmediately);
#endif
}

#ifdef Q_OS_WIN
bool PlatformObserver::winEvent(MSG *message, void *result) {
    Q_UNUSED(result);
    if (message->message == WM_SETTINGCHANGE) {
        LPTSTR str = reinterpret_cast<LPTSTR>(message->lParam);
        if (lstrcmp(str, L"ImmersiveColorSet") == 0) {
            emit colorSchemeChanged(Platform::getWinThemeScheme() == Platform::DarkTheme);
        }
    }
    return false;
}
#endif

#if defined(Q_OS_LINUX) && !defined(Q_OS_ANDROID)
void PlatformObserver::dbusChanged(QString ns, QString key, QDBusVariant value) {
    if (ns == PORTAL_SETTINGS_NAMESPACE && key == COLOR_SCHEME_KEY) {
        QVariant variant = value.variant();
        bool ok;
        int v = variant.toInt(&ok);
        if (!ok) {
            return;
        }
        // 0: No preference
        // 1: Prefer dark
        // 2: Prefer light
        emit colorSchemeChanged(v == 1);
    } else if (ns == "org.kde.kdeglobals.General" && key == "ColorScheme") {
        if (Platform::getLinuxThemeSchemeFromXdgPortal() == Platform::UnknownTheme) {
            QString themeName = value.variant().toString();
            if (themeName.endsWith("Dark", Qt::CaseInsensitive)) {
                emit colorSchemeChanged(true);
            } else {
                emit colorSchemeChanged(false);
            }
        }
    }
}
#endif
