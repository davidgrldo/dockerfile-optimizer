FROM alpine:3.19
RUN cat <<<"inline here-string is not a heredoc"
RUN <<EOF
echo configuring
echo done
EOF
RUN echo finished
