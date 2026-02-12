Name:           goop2
Version:        2.0.0
Release:        1%{?dist}
Summary:        Ephemeral Web - peer-to-peer presence-based web system

License:        MIT
URL:            https://github.com/petervdpas/goop2

BuildArch:      x86_64
AutoReqProv:    no

Requires:       glibc
Requires:       gtk3
Requires:       webkit2gtk4.1
Requires:       libayatana-appindicator-gtk3

%global debug_package %{nil}

%description
Goop2 is a peer-to-peer, presence-based web system for small, personal
websites. Each peer is a self-contained node with its own identity,
content, and optional local web UI.

%install
rm -rf %{buildroot}

mkdir -p %{buildroot}/opt/goop2
install -m 755 %{_sourcedir}/goop2 %{buildroot}/opt/goop2/goop2

mkdir -p %{buildroot}%{_bindir}
install -m 755 %{_sourcedir}/goop2-wrapper.sh %{buildroot}%{_bindir}/goop2

mkdir -p %{buildroot}%{_datadir}/applications
install -m 644 %{_sourcedir}/goop2.desktop %{buildroot}%{_datadir}/applications/goop2.desktop

mkdir -p %{buildroot}%{_datadir}/icons/hicolor/256x256/apps
install -m 644 %{_sourcedir}/goop2.png %{buildroot}%{_datadir}/icons/hicolor/256x256/apps/goop2.png

%files
%dir /opt/goop2
/opt/goop2/goop2
%{_bindir}/goop2
%{_datadir}/applications/goop2.desktop
%{_datadir}/icons/hicolor/256x256/apps/goop2.png

%post
update-desktop-database %{_datadir}/applications 2>/dev/null || true
gtk-update-icon-cache %{_datadir}/icons/hicolor 2>/dev/null || true

%postun
update-desktop-database %{_datadir}/applications 2>/dev/null || true
gtk-update-icon-cache %{_datadir}/icons/hicolor 2>/dev/null || true
