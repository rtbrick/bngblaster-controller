// SPDX-License-Identifier: BSD-3-Clause
// Copyright (C) 2020-2024, RtBrick, Inc.
package controller

// DefaultRepositoryOption helps to configure the Repository with options.
type DefaultRepositoryOption func(repository *DefaultRepository)

// WithConfigFolder is the option to define the config folder for the repository.
func WithConfigFolder(folder string) DefaultRepositoryOption {
	return func(r *DefaultRepository) {
		r.configFolder = folder
	}
}

// WithExecutable the option to define the bngblaster executable.
func WithExecutable(executable string) DefaultRepositoryOption {
	return func(r *DefaultRepository) {
		r.executable = executable
	}
}

// WithUpload is the option to allow file upload.
func WithUpload(upload bool) DefaultRepositoryOption {
	return func(r *DefaultRepository) {
		r.allow_upload = upload
	}
}
