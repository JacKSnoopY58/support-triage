FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY index.html app.css app.js /usr/share/nginx/html/
COPY core/ /usr/share/nginx/html/core/
COPY features/ /usr/share/nginx/html/features/
