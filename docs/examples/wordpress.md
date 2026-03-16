# WordPress + Plugins

Keep WordPress updated with patch-only policy while protecting the
database from unintended upgrades.

## docker-compose.yml

```yaml
services:
  wordpress:
    image: wordpress:6-apache
    ports:
      - "80:80"
    volumes:
      - wp-content:/var/www/html/wp-content
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_NAME: wordpress
      WORDPRESS_DB_USER: wp
      WORDPRESS_DB_PASSWORD_FILE: /run/secrets/db_password
    labels:
      com.updock.lifecycle.pre-update: "wp maintenance-mode activate"
      com.updock.lifecycle.post-update: "wp maintenance-mode deactivate && wp cache flush"

  db:
    image: mariadb:11
    volumes:
      - db-data:/var/lib/mysql

  updock:
    image: ghcr.io/huseyinbabal/updock:latest
    command: ["--lifecycle-hooks"]
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./updock.yml:/etc/updock/updock.yml
    ports:
      - "8080:8080"

volumes:
  wp-content:
  db-data:
```

## updock.yml

```yaml
policies:
  default:
    strategy: patch
    approve: auto
    rollback: on-failure

  locked:
    strategy: pin
    approve: manual

containers:
  wordpress:
    policy: default
    schedule: "03:00-05:00"
  db:
    policy: locked
```

Before updating WordPress, the pre-update hook activates maintenance mode.
After the update, maintenance mode is deactivated and the cache is flushed.
MariaDB is pinned and requires manual approval via the Web UI.
