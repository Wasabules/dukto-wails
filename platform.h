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

#ifndef PLATFORM_H
#define PLATFORM_H

#include <QObject>

#if defined(Q_OS_LINUX) && defined(GSETTINGS_SUPPORT)
class QGSettings;
#endif

class QWindow;

class Platform
{
public:
    static QString getUsername();
    static QString getHostname();
    static QString getPlatformName();
    static QString getAvatarPath();
    static QString getDefaultPath();
    static void setNonClientAreaMode(QWindow *win, bool darkMode);
    static bool isDarkTheme();

private:
    friend class PlatformObserver;

    Platform() = default;
    static QString getSystemUsername();
#if defined(Q_OS_LINUX) && !defined(Q_OS_ANDROID)
    static QString getLinuxAvatarPath();
#elif defined(Q_OS_MAC)
    static QString getMacTempAvatarPath();
#elif defined(Q_OS_WIN)
    static QString getWinTempAvatarPath();
#endif
#if defined(Q_OS_WIN)
    static int isWinDarkTheme();
#elif defined(Q_OS_LINUX) && defined(GSETTINGS_SUPPORT)
    static int isGSettingsDarkTheme(QGSettings *gs);
#endif
#if !defined(Q_OS_ANDROID)
    static QString env(const QString &name);
#endif
};


#ifdef Q_OS_WIN
#include <QWindow>
#endif

class PlatformObserver : public QObject {
    Q_OBJECT
public:
    explicit PlatformObserver(QObject *parent = nullptr);
    virtual ~PlatformObserver();

#if QT_VERSION < QT_VERSION_CHECK(6, 5, 0) && defined(Q_OS_WIN)
    bool winEvent(MSG *message, void *result);
#endif


Q_SIGNALS:
    void colorSchemeChanged(bool darkMode);

private:
    void observe();
#if QT_VERSION < QT_VERSION_CHECK(6, 5, 0)
#if defined(Q_OS_LINUX) && defined(GSETTINGS_SUPPORT)
    QGSettings *gsettings;
    void gsettingsChanged(const QString &key);
#endif
#endif
};


#endif // PLATFORM_H
