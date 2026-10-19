Name:           ryoku-keyring
Version:        0.1
Release:        1%{?dist}
Summary:        Ryoku release signing key for the Ryoku RPM repository
License:        GPL-3.0-or-later
URL:            https://ryoku.dev
Source0:        ryoku-%{version}.tar.gz
BuildArch:      noarch
Requires(post): rpm

%description
The OpenPGP key that signs Ryoku's RPMs and repository metadata. The installer
verifies and imports it first; this package keeps it in place and carries a
rotated key through updates.

%prep
%setup -q -n ryoku-%{version}

%install
install -Dm644 keys/ryoku-release-key.pub.asc %{buildroot}%{_sysconfdir}/pki/rpm-gpg/RPM-GPG-KEY-ryoku

%post
# dnf only trusts keys in the rpm keyring; the file alone would leave a
# rotated key untrusted until something imported it.
rpmkeys --import %{_sysconfdir}/pki/rpm-gpg/RPM-GPG-KEY-ryoku >/dev/null 2>&1 || :

%files
%{_sysconfdir}/pki/rpm-gpg/RPM-GPG-KEY-ryoku
