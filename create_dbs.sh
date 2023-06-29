#!/usr/bin/env bash

set -eu

function create_database_and_user() {
    local database=$1
    local user=$2
    local password=$3

    echo "Creating database with user: $database $user"
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
CREATE USER $user WITH PASSWORD '$password';
CREATE DATABASE $database;
GRANT ALL PRIVILEGES ON DATABASE $database TO $user;
EOSQL
}

if [ -n $POSTGRES_EXTRA_DATABASES ]; then
    echo "Creating multiple databases and users: $POSTGRES_EXTRA_DATABASES"
    for dup in $(echo $POSTGRES_EXTRA_DATABASES | tr ',' ' '); do
        db=$(echo $dup | awk -F":" '{print $1}')
        user=$(echo $dup | awk -F":" '{print $2}')
        password=$(echo $dup | awk -F":" '{print $3}')

        if [ -z "$user"]; then
            user=$db
        fi

        if [ -z "$password" ]; then
            password=$user
        fi

        create_database_and_user $db $user $password
    done

    echo "Created multiple databases"
fi
